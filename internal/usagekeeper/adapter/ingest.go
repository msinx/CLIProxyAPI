package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	internallogging "github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/cpa"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/models"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/repository"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	groupingSalt        = "cliproxyapi-embedded-usage-keeper-v1"
	defaultDrainTimeout = 5 * time.Second
)

type Plugin struct {
	enabled atomic.Bool
	store   atomic.Pointer[usageStore]
	mu      sync.Mutex
}

type usageStore struct {
	db *gorm.DB
	wg sync.WaitGroup
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Enable() {
	if p != nil {
		p.enabled.Store(true)
	}
}

func (p *Plugin) Disable() {
	if p != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.enabled.Store(false)
		p.store.Store(nil)
	}
}

func (p *Plugin) SetStore(store *usageStore) {
	if p != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.store.Store(store)
	}
}

func (p *Plugin) DisableStore(store *usageStore, timeout time.Duration) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if store != nil && p.store.Load() == store {
		p.enabled.Store(false)
		p.store.Store(nil)
	}
	p.mu.Unlock()
	drainStore(store, timeout)
}

func drainStore(store *usageStore, timeout time.Duration) {
	if store == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		store.wg.Wait()
		close(done)
	}()
	if timeout <= 0 {
		<-done
	} else {
		select {
		case <-done:
		case <-time.After(timeout):
			log.Warn("usage keeper plugin drain timeout reached")
		}
	}
}

func (p *Plugin) HandleUsage(ctx context.Context, record coreusage.Record) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.enabled.Load() {
		p.mu.Unlock()
		return
	}
	store := p.store.Load()
	if store == nil {
		p.mu.Unlock()
		return
	}
	store.wg.Add(1)
	p.mu.Unlock()
	defer store.wg.Done()

	event := buildUsageEvent(ctx, record)
	if _, _, err := repository.InsertUsageEvents(store.db, []models.UsageEvent{event}); err != nil {
		log.WithError(err).Warn("usage keeper ingest failed")
	}
}

func buildUsageEvent(ctx context.Context, record coreusage.Record) models.UsageEvent {
	timestamp := record.RequestedAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	timestamp = timestamp.UTC()

	tokens := normalizeTokens(record.Detail)
	provider := firstNonEmpty(record.Provider, "unknown")
	model := firstNonEmpty(record.Model, "unknown")
	authIndex := strings.TrimSpace(record.AuthIndex)
	source := stableUsageSource(record)
	latencyMS := record.Latency.Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	failed := record.Failed
	if !failed {
		failed = !responseWasSuccessful(ctx)
	}
	apiGroupKey := stableGroupKey(record)

	return models.UsageEvent{
		EventKey:        buildRuntimeEventKey(ctx, provider, model, authIndex, timestamp, latencyMS, failed, tokens),
		APIGroupKey:     apiGroupKey,
		Model:           model,
		Timestamp:       timestamp,
		Source:          source,
		AuthIndex:       authIndex,
		Failed:          failed,
		LatencyMS:       latencyMS,
		InputTokens:     tokens.InputTokens,
		OutputTokens:    tokens.OutputTokens,
		ReasoningTokens: tokens.ReasoningTokens,
		CachedTokens:    tokens.CachedTokens,
		TotalTokens:     tokens.TotalTokens,
	}
}

func normalizeTokens(detail coreusage.Detail) cpa.TokenStats {
	tokens := cpa.TokenStats{
		InputTokens:     detail.InputTokens,
		OutputTokens:    detail.OutputTokens,
		ReasoningTokens: detail.ReasoningTokens,
		CachedTokens:    detail.CachedTokens,
		TotalTokens:     detail.TotalTokens,
	}
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens
	}
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens + tokens.CachedTokens
	}
	return tokens
}

func stableGroupKey(record coreusage.Record) string {
	raw := firstNonEmpty(record.APIKey, record.AuthID, record.AuthIndex, record.Provider, "unknown")
	sum := sha256.Sum256([]byte(groupingSalt + "\n" + raw))
	return "grp_" + hex.EncodeToString(sum[:16])
}

func stableUsageSource(record coreusage.Record) string {
	if authIndex := strings.TrimSpace(record.AuthIndex); authIndex != "" {
		return "auth:" + authIndex
	}
	rawKey := strings.TrimSpace(record.APIKey)
	if source := strings.TrimSpace(record.Source); source != "" {
		if rawKey != "" && source == rawKey {
			return stableProviderLookupKey(record.Provider, "", rawKey)
		}
		return source
	}
	if rawKey != "" {
		return stableProviderLookupKey(record.Provider, "", rawKey)
	}
	if authID := strings.TrimSpace(record.AuthID); authID != "" {
		return "auth-id:" + authID
	}
	return ""
}

func buildRuntimeEventKey(ctx context.Context, provider, model, authIndex string, timestamp time.Time, latencyMS int64, failed bool, tokens cpa.TokenStats) string {
	requestID := strings.TrimSpace(internallogging.GetRequestID(ctx))
	if requestID == "" {
		requestID = BuildFallbackEventID(ctx)
	}
	payload := strings.Join([]string{
		requestID,
		provider,
		model,
		authIndex,
		timestamp.UTC().Format(time.RFC3339Nano),
		strconv.FormatInt(latencyMS, 10),
		strconv.FormatBool(failed),
		fmt.Sprintf("%d/%d/%d/%d/%d", tokens.InputTokens, tokens.OutputTokens, tokens.ReasoningTokens, tokens.CachedTokens, tokens.TotalTokens),
	}, "\n")
	sum := sha256.Sum256([]byte(payload))
	return "runtime:" + hex.EncodeToString(sum[:])
}

func BuildFallbackEventID(context.Context) string {
	return fmt.Sprintf("fallback:%d", time.Now().UnixNano())
}

func responseWasSuccessful(ctx context.Context) bool {
	status := internallogging.GetResponseStatus(ctx)
	return status == 0 || status < 400
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
