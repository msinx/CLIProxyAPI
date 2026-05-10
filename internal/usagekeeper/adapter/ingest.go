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

	internallogging "github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/entities"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/repository"
	repodto "github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/repository/dto"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
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
	if _, _, err := repository.InsertUsageEvents(store.db, []entities.UsageEvent{event}); err != nil {
		log.WithError(err).Warn("usage keeper ingest failed")
	}
}

func buildUsageEvent(ctx context.Context, record coreusage.Record) entities.UsageEvent {
	timestamp := record.RequestedAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	timestamp = timestamp.UTC()

	tokens := normalizeTokens(record.Detail)
	provider := firstNonEmpty(record.Provider, "unknown")
	model := firstNonEmpty(record.Model, "unknown")
	authIndex := strings.TrimSpace(record.AuthIndex)
	authType := usageAuthType(record, authIndex)
	if authType == "apikey" && authIndex == "" {
		authIndex = stableAPIKeyAuthIndex(record)
	}
	source := stableUsageSource(record, authType)
	latencyMS := record.Latency.Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	failed := record.Failed
	if !failed {
		failed = !responseWasSuccessful(ctx)
	}
	apiGroupKey := stableGroupKey(record)
	requestID := runtimeRequestID(ctx)

	return entities.UsageEvent{
		EventKey:        buildRuntimeEventKey(requestID, provider, model, authIndex, timestamp, latencyMS, failed, tokens),
		APIGroupKey:     apiGroupKey,
		Provider:        provider,
		Endpoint:        usageEndpoint(record),
		AuthType:        authType,
		RequestID:       requestID,
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

func normalizeTokens(detail coreusage.Detail) repodto.TokenStats {
	tokens := repodto.TokenStats{
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

func stableUsageSource(record coreusage.Record, authType string) string {
	if authIndex := strings.TrimSpace(record.AuthIndex); authType == "oauth" && authIndex != "" {
		return authIndex
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

func stableAPIKeyAuthIndex(record coreusage.Record) string {
	rawKey := strings.TrimSpace(record.APIKey)
	if rawKey == "" {
		return strings.TrimSpace(record.Source)
	}
	return stableProviderLookupKey(record.Provider, "", rawKey)
}

func usageAuthType(record coreusage.Record, authIndex string) string {
	if authType := strings.ToLower(strings.TrimSpace(record.AuthType)); authType != "" {
		switch authType {
		case "oauth", "apikey":
			return authType
		case "api_key":
			return "apikey"
		default:
			return authType
		}
	}
	if strings.TrimSpace(authIndex) != "" {
		return "oauth"
	}
	if strings.TrimSpace(record.Source) != "" || strings.TrimSpace(record.APIKey) != "" {
		return "apikey"
	}
	return ""
}

func usageEndpoint(record coreusage.Record) string {
	source := strings.TrimSpace(record.Source)
	rawKey := strings.TrimSpace(record.APIKey)
	if source == "" || source == rawKey {
		return ""
	}
	if strings.Contains(strings.ToLower(source), "sk-") || strings.Contains(strings.ToLower(source), "aiza") {
		return ""
	}
	return source
}

func runtimeRequestID(ctx context.Context) string {
	requestID := strings.TrimSpace(internallogging.GetRequestID(ctx))
	if requestID == "" {
		requestID = BuildFallbackEventID(ctx)
	}
	return requestID
}

func buildRuntimeEventKey(requestID, provider, model, authIndex string, timestamp time.Time, latencyMS int64, failed bool, tokens repodto.TokenStats) string {
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
