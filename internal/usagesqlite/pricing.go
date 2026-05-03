package usagesqlite

import "strings"

// Default model prices are standard paid-tier USD rates per 1M tokens.
// Keep this table conservative: only include token pricing that can be traced
// to official provider pricing pages.
var defaultModelPrices = map[string]ModelPrice{
	"gpt-5.5":            {PromptPricePer1M: 5, CompletionPricePer1M: 30, CachePricePer1M: 0.5},
	"gpt-5.4":            {PromptPricePer1M: 2.5, CompletionPricePer1M: 15, CachePricePer1M: 0.25},
	"gpt-5.4-mini":       {PromptPricePer1M: 0.75, CompletionPricePer1M: 4.5, CachePricePer1M: 0.075},
	"gpt-5.3-codex":      {PromptPricePer1M: 1.75, CompletionPricePer1M: 14, CachePricePer1M: 0.175},
	"gpt-5.2-codex":      {PromptPricePer1M: 1.75, CompletionPricePer1M: 14, CachePricePer1M: 0.175},
	"gpt-5.2":            {PromptPricePer1M: 1.75, CompletionPricePer1M: 14, CachePricePer1M: 0.175},
	"gpt-5.1-codex-max":  {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
	"gpt-5.1-codex-mini": {PromptPricePer1M: 0.25, CompletionPricePer1M: 2, CachePricePer1M: 0.025},
	"gpt-5.1-codex":      {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
	"gpt-5.1":            {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
	"gpt-5-codex":        {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
	"gpt-5-mini":         {PromptPricePer1M: 0.25, CompletionPricePer1M: 2, CachePricePer1M: 0.025},
	"gpt-5-nano":         {PromptPricePer1M: 0.05, CompletionPricePer1M: 0.4, CachePricePer1M: 0.005},
	"gpt-5":              {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
	"gpt-4.1-mini":       {PromptPricePer1M: 0.4, CompletionPricePer1M: 1.6, CachePricePer1M: 0.1},
	"gpt-4.1-nano":       {PromptPricePer1M: 0.1, CompletionPricePer1M: 0.4, CachePricePer1M: 0.025},
	"gpt-4.1":            {PromptPricePer1M: 2, CompletionPricePer1M: 8, CachePricePer1M: 0.5},
	"gpt-4o-mini":        {PromptPricePer1M: 0.15, CompletionPricePer1M: 0.6, CachePricePer1M: 0.075},
	"gpt-4o":             {PromptPricePer1M: 2.5, CompletionPricePer1M: 10, CachePricePer1M: 1.25},
	"o3":                 {PromptPricePer1M: 2, CompletionPricePer1M: 8, CachePricePer1M: 0.5},
	"o4-mini":            {PromptPricePer1M: 1.1, CompletionPricePer1M: 4.4, CachePricePer1M: 0.275},

	"claude-opus-4-7":   {PromptPricePer1M: 5, CompletionPricePer1M: 25, CachePricePer1M: 0.5},
	"claude-opus-4-1":   {PromptPricePer1M: 15, CompletionPricePer1M: 75, CachePricePer1M: 1.5},
	"claude-opus-4":     {PromptPricePer1M: 15, CompletionPricePer1M: 75, CachePricePer1M: 1.5},
	"claude-sonnet-4-5": {PromptPricePer1M: 3, CompletionPricePer1M: 15, CachePricePer1M: 0.3},
	"claude-sonnet-4":   {PromptPricePer1M: 3, CompletionPricePer1M: 15, CachePricePer1M: 0.3},
	"claude-3-7-sonnet": {PromptPricePer1M: 3, CompletionPricePer1M: 15, CachePricePer1M: 0.3},
	"claude-3-5-sonnet": {PromptPricePer1M: 3, CompletionPricePer1M: 15, CachePricePer1M: 0.3},
	"claude-haiku-4-5":  {PromptPricePer1M: 1, CompletionPricePer1M: 5, CachePricePer1M: 0.1},
	"claude-3-5-haiku":  {PromptPricePer1M: 0.8, CompletionPricePer1M: 4, CachePricePer1M: 0.08},
	"claude-3-haiku":    {PromptPricePer1M: 0.25, CompletionPricePer1M: 1.25, CachePricePer1M: 0.03},

	"gemini-3.1-pro-preview":        {PromptPricePer1M: 2, CompletionPricePer1M: 12, CachePricePer1M: 0.2},
	"gemini-3.1-flash-lite-preview": {PromptPricePer1M: 0.25, CompletionPricePer1M: 1.5, CachePricePer1M: 0.025},
	"gemini-3-flash-preview":        {PromptPricePer1M: 0.5, CompletionPricePer1M: 3, CachePricePer1M: 0.05},
	"gemini-2.5-pro":                {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
	"gemini-2.5-flash-lite":         {PromptPricePer1M: 0.1, CompletionPricePer1M: 0.4, CachePricePer1M: 0.01},
	"gemini-2.5-flash":              {PromptPricePer1M: 0.3, CompletionPricePer1M: 2.5, CachePricePer1M: 0.03},
}

func defaultModelPricesSnapshot() map[string]ModelPrice {
	out := make(map[string]ModelPrice, len(defaultModelPrices))
	for model, price := range defaultModelPrices {
		out[model] = price
	}
	return out
}

func mergeModelPrices(overrides map[string]ModelPrice) map[string]ModelPrice {
	merged := defaultModelPricesSnapshot()
	for model, price := range normalizeModelPrices(overrides) {
		merged[model] = price
	}
	return merged
}

func findModelPrice(prices map[string]ModelPrice, model string) (ModelPrice, bool) {
	model = normalizeModelPriceKey(model)
	if model == "" {
		return ModelPrice{}, false
	}
	if price, ok := prices[model]; ok {
		return price, true
	}

	var bestKey string
	var bestPrice ModelPrice
	for key, price := range prices {
		if len(key) <= len(bestKey) {
			continue
		}
		if modelHasPricePrefix(model, key) {
			bestKey = key
			bestPrice = price
		}
	}
	return bestPrice, bestKey != ""
}

func normalizeModelPriceKey(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return ""
	}
	if idx := strings.IndexByte(model, '('); idx >= 0 {
		model = strings.TrimSpace(model[:idx])
	}
	if idx := strings.LastIndexByte(model, '/'); idx >= 0 {
		model = strings.TrimSpace(model[idx+1:])
	}
	return model
}

func modelHasPricePrefix(model, key string) bool {
	if model == key {
		return true
	}
	if !strings.HasPrefix(model, key) {
		return false
	}
	if len(model) == len(key) {
		return true
	}
	switch model[len(key)] {
	case '-', '.':
		return true
	default:
		return false
	}
}
