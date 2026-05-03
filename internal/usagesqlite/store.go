package usagesqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

type Store struct {
	db     *DB
	prices map[string]ModelPrice
	mu     sync.RWMutex
}

type ModelPrice struct {
	PromptPricePer1M     float64
	CompletionPricePer1M float64
	CachePricePer1M      float64
}

type costAggregate struct {
	Total     float64
	Count     int64
	Unpriced  int64
	Available bool
}

const insertUsageEventSQL = `INSERT OR IGNORE INTO usage_events (
	event_key, request_id, timestamp, provider, model, endpoint, api_group_key, source,
	source_hash, auth_index, auth_id_hash, auth_type, api_key_hash, failed, status_code,
	latency_ms, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func NewStore(db *DB) *Store {
	return &Store{db: db, prices: defaultModelPricesSnapshot()}
}

func (s *Store) SetModelPrices(prices map[string]ModelPrice) {
	if s == nil {
		return
	}
	normalized := normalizeModelPrices(prices)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices = mergeModelPrices(normalized)
}

func (s *Store) InsertEvent(ctx context.Context, event Event) error {
	return s.InsertEvents(ctx, []Event{event})
}

func (s *Store) InsertEvents(ctx context.Context, events []Event) error {
	if s == nil || s.db == nil || s.db.sqlDB == nil {
		return fmt.Errorf("sqlite usage store is not initialized")
	}
	if len(events) == 0 {
		return nil
	}

	normalized := make([]Event, 0, len(events))
	now := time.Now()
	for _, event := range events {
		normalizedEvent, err := normalizeEvent(event, now)
		if err != nil {
			return err
		}
		normalized = append(normalized, normalizedEvent)
	}

	tx, err := s.db.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite usage event batch: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, insertUsageEventSQL)
	if err != nil {
		return fmt.Errorf("prepare sqlite usage event batch: %w", err)
	}
	defer closeStmt(stmt)

	for _, event := range normalized {
		if _, errExec := stmt.ExecContext(ctx,
			event.EventKey,
			event.RequestID,
			event.Timestamp.Unix(),
			event.Provider,
			event.Model,
			event.Endpoint,
			event.APIGroupKey,
			event.Source,
			event.SourceHash,
			event.AuthIndex,
			event.AuthIDHash,
			event.AuthType,
			event.APIKeyHash,
			boolToInt(event.Failed),
			event.StatusCode,
			event.LatencyMS,
			event.InputTokens,
			event.OutputTokens,
			event.ReasoningTokens,
			event.CachedTokens,
			event.TotalTokens,
			event.CreatedAt.Unix(),
		); errExec != nil {
			return fmt.Errorf("insert sqlite usage event batch: %w", errExec)
		}
	}
	if errCommit := tx.Commit(); errCommit != nil {
		return fmt.Errorf("commit sqlite usage event batch: %w", errCommit)
	}
	committed = true
	return nil
}

func (s *Store) GetOverview(ctx context.Context, filter QueryFilter) (Overview, error) {
	overview := Overview{
		HourlySeries: []TimeBucket{},
		DailySeries:  []TimeBucket{},
		Models:       []BreakdownRow{},
		Providers:    []BreakdownRow{},
		APIKeys:      []BreakdownRow{},
		Timezone:     time.Local.String(),
	}
	where, args := buildWhere(filter)
	row := s.db.sqlDB.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN failed = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(reasoning_tokens), 0),
		COALESCE(SUM(cached_tokens), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(AVG(CASE WHEN latency_ms > 0 THEN latency_ms ELSE NULL END), 0)
		FROM usage_events `+where, args...)
	if err := row.Scan(
		&overview.Summary.RequestCount,
		&overview.Summary.SuccessCount,
		&overview.Summary.FailureCount,
		&overview.Summary.InputTokens,
		&overview.Summary.OutputTokens,
		&overview.Summary.ReasoningTokens,
		&overview.Summary.CachedTokens,
		&overview.Summary.TotalTokens,
		&overview.Summary.AverageLatencyMS,
	); err != nil {
		return overview, fmt.Errorf("query sqlite usage overview: %w", err)
	}
	finalizeSummary(&overview.Summary, filter)
	if s.hasModelPrices() {
		costs, errCost := s.costRollups(ctx, filter, "'summary'", false)
		if errCost != nil {
			return overview, fmt.Errorf("query sqlite usage summary cost: %w", errCost)
		}
		applyCostAggregateToSummary(&overview.Summary, costs["summary"])
	}

	var err error
	overview.HourlySeries, err = s.listBuckets(ctx, filter, 3600)
	if err != nil {
		return overview, err
	}
	overview.DailySeries, err = s.listBuckets(ctx, filter, 86400)
	if err != nil {
		return overview, err
	}
	overview.Models, err = s.breakdown(ctx, filter, "model")
	if err != nil {
		return overview, err
	}
	overview.Providers, err = s.breakdown(ctx, filter, "provider")
	if err != nil {
		return overview, err
	}
	overview.APIKeys, err = s.breakdown(ctx, filter, "api_group_key")
	if err != nil {
		return overview, err
	}
	overview.RangeStart = filter.StartTime
	overview.RangeEnd = filter.EndTime
	return overview, nil
}

func (s *Store) GetAnalysis(ctx context.Context, filter QueryFilter) ([]BreakdownRow, []BreakdownRow, error) {
	providers, err := s.breakdown(ctx, filter, "provider")
	if err != nil {
		return nil, nil, err
	}
	models, err := s.breakdown(ctx, filter, "model")
	if err != nil {
		return nil, nil, err
	}
	return providers, models, nil
}

func (s *Store) ListEvents(ctx context.Context, filter QueryFilter) (EventsPage, error) {
	page := normalizePage(filter.Page)
	pageSize := normalizePageSize(filter.PageSize)
	filter.Page = page
	filter.PageSize = pageSize
	where, args := buildWhere(filter)

	var total int64
	if err := s.db.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_events `+where, args...).Scan(&total); err != nil {
		return EventsPage{}, fmt.Errorf("count sqlite usage events: %w", err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.sqlDB.QueryContext(ctx, `SELECT id, request_id, timestamp, provider, model, endpoint,
		api_group_key, source, source_hash, auth_index, auth_id_hash, auth_type, failed, status_code,
		latency_ms, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens, created_at
		FROM usage_events `+where+` ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return EventsPage{}, fmt.Errorf("list sqlite usage events: %w", err)
	}
	defer closeRows(rows)

	events := make([]EventView, 0, pageSize)
	for rows.Next() {
		var event Event
		var failed int
		var ts, created int64
		if errScan := rows.Scan(
			&event.ID, &event.RequestID, &ts, &event.Provider, &event.Model, &event.Endpoint,
			&event.APIGroupKey, &event.Source, &event.SourceHash, &event.AuthIndex, &event.AuthIDHash,
			&event.AuthType, &failed, &event.StatusCode, &event.LatencyMS, &event.InputTokens,
			&event.OutputTokens, &event.ReasoningTokens, &event.CachedTokens, &event.TotalTokens, &created,
		); errScan != nil {
			return EventsPage{}, fmt.Errorf("scan sqlite usage event: %w", errScan)
		}
		event.Timestamp = time.Unix(ts, 0).UTC()
		event.CreatedAt = time.Unix(created, 0).UTC()
		event.Failed = failed != 0
		events = append(events, s.eventView(event))
	}
	if errRows := rows.Err(); errRows != nil {
		return EventsPage{}, fmt.Errorf("iterate sqlite usage events: %w", errRows)
	}

	models, err := s.listDistinct(ctx, filter, "model")
	if err != nil {
		return EventsPage{}, err
	}
	providers, err := s.listDistinct(ctx, filter, "provider")
	if err != nil {
		return EventsPage{}, err
	}
	sources, err := s.listSources(ctx, filter)
	if err != nil {
		return EventsPage{}, err
	}
	return EventsPage{
		Events:     events,
		TotalCount: total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
		Models:     models,
		Providers:  providers,
		Sources:    sources,
	}, nil
}

func (s *Store) ListCredentials(ctx context.Context, filter QueryFilter) ([]CredentialRow, error) {
	where, args := buildWhere(filter)
	rows, err := s.db.sqlDB.QueryContext(ctx, `SELECT
		provider,
		source,
		source_hash,
		auth_index,
		auth_id_hash,
		auth_type,
		COUNT(*) AS request_count,
		COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0) AS success_count,
		COALESCE(SUM(CASE WHEN failed = 1 THEN 1 ELSE 0 END), 0) AS failure_count,
		COALESCE(SUM(input_tokens), 0) AS input_tokens,
		COALESCE(SUM(output_tokens), 0) AS output_tokens,
		COALESCE(SUM(reasoning_tokens), 0) AS reasoning_tokens,
		COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		COALESCE(SUM(total_tokens), 0) AS total_tokens,
		COALESCE(AVG(CASE WHEN latency_ms > 0 THEN latency_ms ELSE NULL END), 0) AS average_latency_ms
		FROM usage_events `+where+`
		GROUP BY provider, source_hash, auth_index, auth_id_hash, auth_type
		ORDER BY request_count DESC, provider ASC, source_hash ASC, auth_index ASC, auth_id_hash ASC
		LIMIT 500`, args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite usage credentials: %w", err)
	}
	defer closeRows(rows)

	out := []CredentialRow{}
	for rows.Next() {
		var row CredentialRow
		var source string
		if errScan := rows.Scan(
			&row.Provider,
			&source,
			&row.SourceHash,
			&row.AuthIndex,
			&row.AuthIDHash,
			&row.AuthType,
			&row.RequestCount,
			&row.SuccessCount,
			&row.FailureCount,
			&row.InputTokens,
			&row.OutputTokens,
			&row.ReasoningTokens,
			&row.CachedTokens,
			&row.TotalTokens,
			&row.AverageLatencyMS,
		); errScan != nil {
			return nil, fmt.Errorf("scan sqlite usage credential: %w", errScan)
		}
		row.SourceDisplay = credentialSourceDisplay(row.Provider, row.AuthType, row.AuthIndex, source)
		row.SourceType = normalizeSourceType(row.AuthType)
		row.SourceKey = row.SourceHash
		if row.RequestCount > 0 {
			row.SuccessRate = float64(row.SuccessCount) / float64(row.RequestCount)
		}
		out = append(out, row)
	}
	if errRows := rows.Err(); errRows != nil {
		return nil, fmt.Errorf("iterate sqlite usage credentials: %w", errRows)
	}
	return out, nil
}

func (s *Store) DeleteBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.sqlDB.ExecContext(ctx, `DELETE FROM usage_events WHERE timestamp < ?`, cutoff.Unix())
	if err != nil {
		return 0, fmt.Errorf("delete old sqlite usage events: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted sqlite usage events count: %w", err)
	}
	return rows, nil
}

func (s *Store) listBuckets(ctx context.Context, filter QueryFilter, seconds int64) ([]TimeBucket, error) {
	where, args := buildWhere(filter)
	rows, err := s.db.sqlDB.QueryContext(ctx, fmt.Sprintf(`SELECT
		(timestamp / %d) * %d AS bucket,
		COUNT(*),
		COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN failed = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(reasoning_tokens), 0),
		COALESCE(SUM(cached_tokens), 0),
		COALESCE(SUM(total_tokens), 0)
		FROM usage_events %s GROUP BY bucket ORDER BY bucket ASC`, seconds, seconds, where), args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite usage buckets: %w", err)
	}

	out := []TimeBucket{}
	for rows.Next() {
		var bucketUnix int64
		var bucket TimeBucket
		if errScan := rows.Scan(&bucketUnix, &bucket.RequestCount, &bucket.SuccessCount, &bucket.FailureCount,
			&bucket.InputTokens, &bucket.OutputTokens, &bucket.ReasoningTokens, &bucket.CachedTokens, &bucket.TotalTokens); errScan != nil {
			return nil, fmt.Errorf("scan sqlite usage bucket: %w", errScan)
		}
		bucket.Bucket = time.Unix(bucketUnix, 0).UTC()
		out = append(out, bucket)
	}
	if errRows := rows.Err(); errRows != nil {
		closeRows(rows)
		return nil, fmt.Errorf("iterate sqlite usage buckets: %w", errRows)
	}
	closeRows(rows)
	if !s.hasModelPrices() {
		return out, nil
	}
	costs, errCost := s.costRollups(ctx, filter, fmt.Sprintf("CAST((timestamp / %d) * %d AS TEXT)", seconds, seconds), false)
	if errCost != nil {
		return nil, fmt.Errorf("query sqlite usage bucket cost: %w", errCost)
	}
	for i := range out {
		applyCostAggregateToBucket(&out[i], costs[fmt.Sprintf("%d", out[i].Bucket.Unix())])
	}
	return out, nil
}

func (s *Store) breakdown(ctx context.Context, filter QueryFilter, column string) ([]BreakdownRow, error) {
	if !allowedBreakdownColumn(column) {
		return nil, fmt.Errorf("unsupported sqlite usage breakdown column %q", column)
	}
	where, args := buildWhere(filter)
	rows, err := s.db.sqlDB.QueryContext(ctx, fmt.Sprintf(`SELECT
		TRIM(%s) AS key,
		COUNT(*) AS request_count,
		COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0) AS success_count,
		COALESCE(SUM(CASE WHEN failed = 1 THEN 1 ELSE 0 END), 0) AS failure_count,
		COALESCE(SUM(input_tokens), 0) AS input_tokens,
		COALESCE(SUM(output_tokens), 0) AS output_tokens,
		COALESCE(SUM(reasoning_tokens), 0) AS reasoning_tokens,
		COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		COALESCE(SUM(total_tokens), 0) AS total_tokens,
		COALESCE(AVG(CASE WHEN latency_ms > 0 THEN latency_ms ELSE NULL END), 0) AS average_latency_ms
		FROM usage_events %s GROUP BY TRIM(%s) HAVING key != '' ORDER BY request_count DESC, key ASC LIMIT 100`, column, where, column), args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite usage breakdown: %w", err)
	}

	out := []BreakdownRow{}
	for rows.Next() {
		var row BreakdownRow
		if errScan := rows.Scan(&row.Key, &row.RequestCount, &row.SuccessCount, &row.FailureCount,
			&row.InputTokens, &row.OutputTokens, &row.ReasoningTokens, &row.CachedTokens,
			&row.TotalTokens, &row.AverageLatencyMS); errScan != nil {
			return nil, fmt.Errorf("scan sqlite usage breakdown: %w", errScan)
		}
		if row.RequestCount > 0 {
			row.SuccessRate = float64(row.SuccessCount) / float64(row.RequestCount)
		}
		row.DisplayName = row.Key
		out = append(out, row)
	}
	if errRows := rows.Err(); errRows != nil {
		closeRows(rows)
		return nil, fmt.Errorf("iterate sqlite usage breakdown: %w", errRows)
	}
	closeRows(rows)
	if !s.hasModelPrices() {
		return out, nil
	}
	costs, errCost := s.costRollups(ctx, filter, fmt.Sprintf("TRIM(%s)", column), true)
	if errCost != nil {
		return nil, fmt.Errorf("query sqlite usage breakdown cost: %w", errCost)
	}
	for i := range out {
		applyCostAggregateToBreakdown(&out[i], costs[out[i].Key])
	}
	return out, nil
}

func (s *Store) listDistinct(ctx context.Context, filter QueryFilter, column string) ([]string, error) {
	if !allowedBreakdownColumn(column) {
		return nil, fmt.Errorf("unsupported sqlite usage distinct column %q", column)
	}
	where, args := buildWhere(filter)
	rows, err := s.db.sqlDB.QueryContext(ctx, fmt.Sprintf(`SELECT DISTINCT TRIM(%s) FROM usage_events %s
		AND TRIM(%s) != '' ORDER BY TRIM(%s) ASC LIMIT 500`, column, where, column, column), args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite usage filter options: %w", err)
	}
	defer closeRows(rows)
	out := []string{}
	for rows.Next() {
		var value string
		if errScan := rows.Scan(&value); errScan != nil {
			return nil, fmt.Errorf("scan sqlite usage filter option: %w", errScan)
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *Store) listSources(ctx context.Context, filter QueryFilter) ([]SourceOption, error) {
	where, args := buildWhere(filter)
	rows, err := s.db.sqlDB.QueryContext(ctx, `SELECT source, source_hash, provider, auth_type, auth_index
		FROM usage_events WHERE id IN (
			SELECT MAX(id) FROM usage_events `+where+` AND source_hash != '' GROUP BY source_hash
		) ORDER BY timestamp DESC LIMIT 500`, args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite usage source options: %w", err)
	}
	defer closeRows(rows)
	out := []SourceOption{}
	for rows.Next() {
		var source, hash, provider, authType, authIndex string
		if errScan := rows.Scan(&source, &hash, &provider, &authType, &authIndex); errScan != nil {
			return nil, fmt.Errorf("scan sqlite usage source option: %w", errScan)
		}
		out = append(out, SourceOption{
			Display:    credentialSourceDisplay(provider, authType, authIndex, source),
			Hash:       hash,
			SourceType: normalizeSourceType(authType),
			SourceKey:  hash,
		})
	}
	return out, rows.Err()
}

func buildWhere(filter QueryFilter) (string, []any) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 8)
	if filter.StartTime != nil {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, filter.StartTime.Unix())
	}
	if filter.EndTime != nil {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, filter.EndTime.Unix())
	}
	if filter.Model != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, filter.Model)
	}
	if filter.Provider != "" {
		clauses = append(clauses, "provider = ?")
		args = append(args, filter.Provider)
	}
	if filter.SourceHash != "" {
		clauses = append(clauses, "source_hash = ?")
		args = append(args, filter.SourceHash)
	}
	if filter.AuthIndex != "" {
		clauses = append(clauses, "auth_index = ?")
		args = append(args, filter.AuthIndex)
	}
	if filter.AuthIDHash != "" {
		clauses = append(clauses, "auth_id_hash = ?")
		args = append(args, filter.AuthIDHash)
	}
	switch filter.Result {
	case "success":
		clauses = append(clauses, "failed = 0")
	case "failed":
		clauses = append(clauses, "failed = 1")
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (s *Store) eventView(event Event) EventView {
	view := EventView{
		ID:              event.ID,
		RequestID:       event.RequestID,
		Timestamp:       event.Timestamp,
		Provider:        event.Provider,
		Model:           event.Model,
		Endpoint:        event.Endpoint,
		APIGroupKey:     event.APIGroupKey,
		SourceDisplay:   credentialSourceDisplay(event.Provider, event.AuthType, event.AuthIndex, event.Source),
		SourceType:      normalizeSourceType(event.AuthType),
		SourceKey:       event.SourceHash,
		SourceHash:      event.SourceHash,
		AuthIndex:       event.AuthIndex,
		AuthIDHash:      event.AuthIDHash,
		AuthType:        event.AuthType,
		Failed:          event.Failed,
		StatusCode:      event.StatusCode,
		LatencyMS:       event.LatencyMS,
		InputTokens:     event.InputTokens,
		OutputTokens:    event.OutputTokens,
		ReasoningTokens: event.ReasoningTokens,
		CachedTokens:    event.CachedTokens,
		TotalTokens:     event.TotalTokens,
		CreatedAt:       event.CreatedAt,
	}
	view.EstimatedCost, view.CostAvailable = s.eventCost(event)
	return view
}

func normalizeModelPrices(prices map[string]ModelPrice) map[string]ModelPrice {
	if len(prices) == 0 {
		return nil
	}
	normalized := make(map[string]ModelPrice, len(prices))
	for model, price := range prices {
		model = normalizeModelPriceKey(model)
		if model == "" {
			continue
		}
		if price.PromptPricePer1M < 0 {
			price.PromptPricePer1M = 0
		}
		if price.CompletionPricePer1M < 0 {
			price.CompletionPricePer1M = 0
		}
		if price.CachePricePer1M < 0 {
			price.CachePricePer1M = 0
		}
		normalized[model] = price
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func (s *Store) modelPricesSnapshot() map[string]ModelPrice {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.prices) == 0 {
		return nil
	}
	out := make(map[string]ModelPrice, len(s.prices))
	for model, price := range s.prices {
		out[model] = price
	}
	return out
}

func (s *Store) hasModelPrices() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.prices) > 0
}

func (s *Store) eventCost(event Event) (float64, bool) {
	prices := s.modelPricesSnapshot()
	return eventCostWithPrices(event, prices)
}

func eventCostWithPrices(event Event, prices map[string]ModelPrice) (float64, bool) {
	price, ok := findModelPrice(prices, event.Model)
	if !ok {
		return 0, false
	}
	cost := float64(event.InputTokens)/1_000_000*price.PromptPricePer1M +
		float64(event.OutputTokens)/1_000_000*price.CompletionPricePer1M +
		float64(event.CachedTokens)/1_000_000*price.CachePricePer1M
	return cost, true
}

func (s *Store) costRollups(ctx context.Context, filter QueryFilter, keyExpr string, requireNonEmptyKey bool) (map[string]costAggregate, error) {
	prices := s.modelPricesSnapshot()
	where, args := buildWhere(filter)
	if requireNonEmptyKey {
		where += fmt.Sprintf(" AND %s != ''", keyExpr)
	}
	query := fmt.Sprintf(`SELECT %s AS cost_key,
		TRIM(model) AS model_key,
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cached_tokens), 0),
		COUNT(*)
		FROM usage_events %s GROUP BY cost_key, model_key`, keyExpr, where)
	rows, err := s.db.sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	out := map[string]costAggregate{}
	for rows.Next() {
		var key string
		var event Event
		var count int64
		if errScan := rows.Scan(&key, &event.Model, &event.InputTokens, &event.OutputTokens, &event.CachedTokens, &count); errScan != nil {
			return nil, errScan
		}
		aggregate := out[key]
		aggregate.Count += count
		cost, ok := eventCostWithPrices(event, prices)
		if !ok {
			aggregate.Unpriced += count
		} else {
			aggregate.Total += cost
		}
		aggregate.Available = aggregate.Count > 0 && aggregate.Unpriced == 0
		out[key] = aggregate
	}
	if errRows := rows.Err(); errRows != nil {
		return nil, errRows
	}
	return out, nil
}

func applyCostAggregateToSummary(summary *Summary, aggregate costAggregate) {
	if summary == nil || aggregate.Count == 0 {
		return
	}
	summary.TotalCost = aggregate.Total
	summary.CostAvailable = aggregate.Available
}

func applyCostAggregateToBucket(bucket *TimeBucket, aggregate costAggregate) {
	if bucket == nil || aggregate.Count == 0 {
		return
	}
	bucket.TotalCost = aggregate.Total
	bucket.CostAvailable = aggregate.Available
}

func applyCostAggregateToBreakdown(row *BreakdownRow, aggregate costAggregate) {
	if row == nil || aggregate.Count == 0 {
		return
	}
	row.TotalCost = aggregate.Total
	row.CostAvailable = aggregate.Available
}

func maskSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	at := strings.IndexByte(source, '@')
	if at > 0 {
		local := source[:at]
		if len(local) == 1 {
			return local + "***" + source[at:]
		}
		return local[:1] + "***" + source[at:]
	}
	if len(source) <= 8 {
		if len(source) <= 4 {
			return "****"
		}
		return source[:1] + "..." + source[len(source)-1:]
	}
	return source[:4] + "..." + source[len(source)-4:]
}

func credentialSourceDisplay(provider, authType, authIndex, source string) string {
	parts := make([]string, 0, 3)
	if displayProvider := providerDisplayName(provider); displayProvider != "" {
		parts = append(parts, displayProvider)
	}
	if displayType := authTypeDisplayName(authType); displayType != "" {
		parts = append(parts, displayType)
	}
	if masked := maskSource(source); masked != "" {
		parts = append(parts, masked)
	} else if idx := strings.TrimSpace(authIndex); idx != "" {
		parts = append(parts, "credential #"+idx)
	}
	return strings.Join(parts, " · ")
}

func providerDisplayName(provider string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(provider)); normalized {
	case "":
		return ""
	case "openai":
		return "OpenAI"
	case "claude":
		return "Claude"
	case "gemini":
		return "Gemini"
	case "gemini-cli":
		return "Gemini CLI"
	case "codex":
		return "Codex"
	case "vertex":
		return "Vertex"
	case "antigravity":
		return "Antigravity"
	case "kimi":
		return "Kimi"
	case "aistudio":
		return "AI Studio"
	default:
		return provider
	}
}

func authTypeDisplayName(authType string) string {
	switch normalizeSourceType(authType) {
	case "":
		return ""
	case "oauth":
		return "OAuth"
	case "api_key":
		return "API key"
	default:
		return strings.TrimSpace(authType)
	}
}

func normalizeSourceType(authType string) string {
	normalized := strings.ToLower(strings.TrimSpace(authType))
	if normalized == "apikey" || normalized == "api-key" {
		return "api_key"
	}
	return normalized
}

func finalizeSummary(summary *Summary, filter QueryFilter) {
	if summary == nil {
		return
	}
	if summary.RequestCount > 0 {
		summary.SuccessRate = float64(summary.SuccessCount) / float64(summary.RequestCount)
	}
	if filter.StartTime != nil && filter.EndTime != nil && filter.EndTime.After(*filter.StartTime) {
		minutes := filter.EndTime.Sub(*filter.StartTime).Minutes()
		if minutes > 0 {
			summary.RPM = float64(summary.RequestCount) / minutes
			summary.TPM = float64(summary.TotalTokens) / minutes
		}
	}
}

func allowedBreakdownColumn(column string) bool {
	switch column {
	case "model", "provider", "api_group_key", "source_hash", "auth_index", "auth_id_hash":
		return true
	default:
		return false
	}
}

func normalizePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizePageSize(pageSize int) int {
	if pageSize <= 0 {
		return 50
	}
	if pageSize > 500 {
		return 500
	}
	return pageSize
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeEvent(event Event, now time.Time) (Event, error) {
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	event.EventKey = strings.TrimSpace(event.EventKey)
	if event.EventKey == "" {
		return Event{}, fmt.Errorf("usage event key is empty")
	}
	if event.TotalTokens == 0 {
		event.TotalTokens = event.InputTokens + event.OutputTokens + event.ReasoningTokens
	}
	if event.TotalTokens == 0 {
		event.TotalTokens = event.InputTokens + event.OutputTokens + event.ReasoningTokens + event.CachedTokens
	}
	return event, nil
}

func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}

func closeStmt(stmt *sql.Stmt) {
	_ = stmt.Close()
}
