package usagesqlite

import "time"

// Event is the persisted usage record. Raw Source is kept internal and is not
// returned directly by management API responses.
type Event struct {
	ID              int64     `json:"id"`
	EventKey        string    `json:"event_key"`
	RequestID       string    `json:"request_id"`
	Timestamp       time.Time `json:"timestamp"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	Endpoint        string    `json:"endpoint"`
	APIGroupKey     string    `json:"api_group_key"`
	Source          string    `json:"-"`
	SourceHash      string    `json:"source_hash"`
	AuthIndex       string    `json:"auth_index"`
	AuthIDHash      string    `json:"auth_id_hash"`
	AuthType        string    `json:"auth_type"`
	APIKeyHash      string    `json:"api_key_hash"`
	Failed          bool      `json:"failed"`
	StatusCode      int       `json:"status_code"`
	LatencyMS       int64     `json:"latency_ms"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	ReasoningTokens int64     `json:"reasoning_tokens"`
	CachedTokens    int64     `json:"cached_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	CreatedAt       time.Time `json:"created_at"`
}

type EventView struct {
	ID              int64     `json:"id"`
	RequestID       string    `json:"request_id"`
	Timestamp       time.Time `json:"timestamp"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	Endpoint        string    `json:"endpoint"`
	APIGroupKey     string    `json:"api_group_key"`
	SourceDisplay   string    `json:"source_display"`
	SourceType      string    `json:"source_type,omitempty"`
	SourceKey       string    `json:"source_key,omitempty"`
	SourceHash      string    `json:"source_hash"`
	AuthIndex       string    `json:"auth_index"`
	AuthIDHash      string    `json:"auth_id_hash"`
	AuthType        string    `json:"auth_type"`
	Failed          bool      `json:"failed"`
	StatusCode      int       `json:"status_code"`
	LatencyMS       int64     `json:"latency_ms"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	ReasoningTokens int64     `json:"reasoning_tokens"`
	CachedTokens    int64     `json:"cached_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	EstimatedCost   float64   `json:"estimated_cost,omitempty"`
	CostAvailable   bool      `json:"cost_available,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type QueryFilter struct {
	StartTime  *time.Time
	EndTime    *time.Time
	Range      string
	Model      string
	Provider   string
	SourceHash string
	AuthIndex  string
	AuthIDHash string
	Result     string
	Limit      int
	Page       int
	PageSize   int
}

type Summary struct {
	RequestCount     int64   `json:"request_count"`
	SuccessCount     int64   `json:"success_count"`
	FailureCount     int64   `json:"failure_count"`
	SuccessRate      float64 `json:"success_rate"`
	TotalTokens      int64   `json:"total_tokens"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	AverageLatencyMS float64 `json:"average_latency_ms"`
	RPM              float64 `json:"rpm"`
	TPM              float64 `json:"tpm"`
	TotalCost        float64 `json:"total_cost,omitempty"`
	CostAvailable    bool    `json:"cost_available,omitempty"`
}

type BreakdownRow struct {
	Key              string  `json:"key"`
	DisplayName      string  `json:"display_name,omitempty"`
	RequestCount     int64   `json:"request_count"`
	SuccessCount     int64   `json:"success_count"`
	FailureCount     int64   `json:"failure_count"`
	SuccessRate      float64 `json:"success_rate"`
	TotalTokens      int64   `json:"total_tokens"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	AverageLatencyMS float64 `json:"average_latency_ms"`
	TotalCost        float64 `json:"total_cost,omitempty"`
	CostAvailable    bool    `json:"cost_available,omitempty"`
}

type CredentialRow struct {
	Provider         string  `json:"provider,omitempty"`
	SourceDisplay    string  `json:"source_display"`
	SourceType       string  `json:"source_type,omitempty"`
	SourceKey        string  `json:"source_key,omitempty"`
	SourceHash       string  `json:"source_hash"`
	AuthIndex        string  `json:"auth_index"`
	AuthIDHash       string  `json:"auth_id_hash"`
	AuthType         string  `json:"auth_type"`
	RequestCount     int64   `json:"request_count"`
	SuccessCount     int64   `json:"success_count"`
	FailureCount     int64   `json:"failure_count"`
	SuccessRate      float64 `json:"success_rate"`
	TotalTokens      int64   `json:"total_tokens"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	AverageLatencyMS float64 `json:"average_latency_ms"`
}

type TimeBucket struct {
	Bucket          time.Time `json:"bucket"`
	RequestCount    int64     `json:"request_count"`
	SuccessCount    int64     `json:"success_count"`
	FailureCount    int64     `json:"failure_count"`
	TotalTokens     int64     `json:"total_tokens"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	ReasoningTokens int64     `json:"reasoning_tokens"`
	CachedTokens    int64     `json:"cached_tokens"`
	TotalCost       float64   `json:"total_cost,omitempty"`
	CostAvailable   bool      `json:"cost_available,omitempty"`
}

type Overview struct {
	Summary      Summary        `json:"summary"`
	HourlySeries []TimeBucket   `json:"hourly_series"`
	DailySeries  []TimeBucket   `json:"daily_series"`
	Models       []BreakdownRow `json:"models"`
	Providers    []BreakdownRow `json:"providers"`
	APIKeys      []BreakdownRow `json:"api_keys"`
	RangeStart   *time.Time     `json:"range_start,omitempty"`
	RangeEnd     *time.Time     `json:"range_end,omitempty"`
	Timezone     string         `json:"timezone"`
}

type EventsPage struct {
	Events     []EventView    `json:"events"`
	TotalCount int64          `json:"total_count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
	Models     []string       `json:"models"`
	Providers  []string       `json:"providers"`
	Sources    []SourceOption `json:"sources"`
}

type SourceOption struct {
	Display    string `json:"display"`
	Hash       string `json:"hash"`
	SourceType string `json:"source_type,omitempty"`
	SourceKey  string `json:"source_key,omitempty"`
}
