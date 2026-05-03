package usagesqlite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	internallogging "github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
)

const (
	defaultBufferSize    = 4096
	defaultBatchSize     = 100
	defaultFlushInterval = time.Second
)

type Plugin struct {
	store         *Store
	salt          string
	queue         chan Event
	batchSize     int
	flushInterval time.Duration
	enabled       atomic.Bool
	stopped       atomic.Bool
	dropped       atomic.Uint64
	cancel        context.CancelFunc
	done          chan struct{}
	stopOnce      sync.Once
	startOnce     sync.Once
}

type PluginOptions struct {
	Enabled       bool
	BufferSize    int
	BatchSize     int
	FlushInterval time.Duration
}

func NewPlugin(store *Store, salt string, opts PluginOptions) *Plugin {
	bufferSize := opts.BufferSize
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	flushInterval := opts.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	p := &Plugin{
		store:         store,
		salt:          strings.TrimSpace(salt),
		queue:         make(chan Event, bufferSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		done:          make(chan struct{}),
	}
	p.enabled.Store(opts.Enabled)
	return p
}

func (p *Plugin) Start(ctx context.Context) {
	if p == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		p.cancel = cancel
		go p.run(runCtx)
	})
}

func (p *Plugin) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		p.enabled.Store(false)
		if p.cancel != nil {
			p.cancel()
		}
	})
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Plugin) SetEnabled(enabled bool) {
	if p != nil {
		p.enabled.Store(enabled)
	}
}

func (p *Plugin) DroppedCount() uint64 {
	if p == nil {
		return 0
	}
	return p.dropped.Load()
}

func (p *Plugin) HandleUsage(ctx context.Context, record coreusage.Record) {
	if p == nil || p.stopped.Load() || !p.enabled.Load() {
		return
	}
	event := p.recordToEvent(ctx, record)
	select {
	case p.queue <- event:
	default:
		dropped := p.dropped.Add(1)
		if dropped == 1 || dropped%1000 == 0 {
			log.WithField("dropped", dropped).Warn("sqlite usage queue full; dropping usage event")
		}
	}
}

func (p *Plugin) run(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.flushInterval)
	defer ticker.Stop()

	batch := make([]Event, 0, p.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := p.store.InsertEvents(context.Background(), batch); err != nil {
			log.WithError(err).Warn("failed to write sqlite usage event batch")
		}
		batch = batch[:0]
	}

	for {
		select {
		case event := <-p.queue:
			batch = append(batch, event)
			if len(batch) >= p.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			for {
				select {
				case event := <-p.queue:
					batch = append(batch, event)
					if len(batch) >= p.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (p *Plugin) recordToEvent(ctx context.Context, record coreusage.Record) Event {
	timestamp := record.RequestedAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	provider := defaultString(record.Provider, "unknown")
	model := defaultString(record.Model, "unknown")
	authType := defaultString(record.AuthType, "unknown")
	requestID := strings.TrimSpace(internallogging.GetRequestID(ctx))
	statusCode := internallogging.GetResponseStatus(ctx)
	failed := record.Failed
	if !failed && statusCode >= 400 {
		failed = true
	}
	totalTokens := record.Detail.TotalTokens
	if totalTokens == 0 {
		totalTokens = record.Detail.InputTokens + record.Detail.OutputTokens + record.Detail.ReasoningTokens
	}
	if totalTokens == 0 {
		totalTokens = record.Detail.InputTokens + record.Detail.OutputTokens + record.Detail.ReasoningTokens + record.Detail.CachedTokens
	}
	sourceHash := p.shortHMAC("src", record.Source)
	authIDHash := p.shortHMAC("auth", record.AuthID)
	apiKeyHash := p.shortHMAC("key", record.APIKey)
	apiGroupKey := p.apiGroupKey(record.APIKey, provider, internallogging.GetEndpoint(ctx))
	eventKey := requestID
	if eventKey != "" {
		eventKey = eventKey + "_" + p.shortHMAC("", p.eventKeyMaterial(provider, model, record, failed))
	} else {
		eventKey = "evt_" + p.shortHMAC("", fmt.Sprintf("%d|%s", timestamp.UnixNano(), p.eventKeyMaterial(provider, model, record, failed)))
	}

	return Event{
		EventKey:        eventKey,
		RequestID:       requestID,
		Timestamp:       timestamp,
		Provider:        provider,
		Model:           model,
		Endpoint:        strings.TrimSpace(internallogging.GetEndpoint(ctx)),
		APIGroupKey:     apiGroupKey,
		Source:          strings.TrimSpace(record.Source),
		SourceHash:      sourceHash,
		AuthIndex:       strings.TrimSpace(record.AuthIndex),
		AuthIDHash:      authIDHash,
		AuthType:        authType,
		APIKeyHash:      apiKeyHash,
		Failed:          failed,
		StatusCode:      statusCode,
		LatencyMS:       record.Latency.Milliseconds(),
		InputTokens:     record.Detail.InputTokens,
		OutputTokens:    record.Detail.OutputTokens,
		ReasoningTokens: record.Detail.ReasoningTokens,
		CachedTokens:    record.Detail.CachedTokens,
		TotalTokens:     totalTokens,
		CreatedAt:       time.Now(),
	}
}

func (p *Plugin) apiGroupKey(apiKey, provider, endpoint string) string {
	if strings.TrimSpace(apiKey) != "" {
		return "key:" + p.shortHMAC("", apiKey)
	}
	if strings.TrimSpace(provider) != "" && provider != "unknown" {
		return "provider:" + provider
	}
	if strings.TrimSpace(endpoint) != "" {
		return "endpoint:" + strings.TrimSpace(endpoint)
	}
	return "unknown"
}

func (p *Plugin) eventKeyMaterial(provider, model string, record coreusage.Record, failed bool) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%d|%d|%t",
		provider,
		model,
		record.Source,
		record.AuthIndex,
		record.Detail.InputTokens,
		record.Detail.OutputTokens,
		record.Detail.ReasoningTokens,
		record.Detail.CachedTokens,
		record.Detail.TotalTokens,
		failed,
	)
}

func (p *Plugin) shortHMAC(prefix, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	key := []byte(p.salt)
	if len(key) == 0 {
		key = []byte("cliproxy-usage")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	short := hex.EncodeToString(mac.Sum(nil))[:12]
	if prefix == "" {
		return short
	}
	return prefix + "_" + short
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
