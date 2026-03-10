package repository

// LogStatistics contains aggregated log statistics.
type LogStatistics struct {
	TotalRequests        int64                `json:"total_requests"`
	TotalCost            float64              `json:"total_cost"`
	AvgLatency           float64              `json:"avg_latency"`
	SuccessRate          float64              `json:"success_rate"`
	CacheHitRate         float64              `json:"cache_hit_rate"`
	TotalCacheReadTokens int64                `json:"total_cache_read_tokens"`
	TotalInputTokens     int64                `json:"total_input_tokens"`
	TotalOutputTokens    int64                `json:"total_output_tokens"`
	ByModel              []ModelStatistics    `json:"by_model"`
	ByEndpoint           []EndpointStatistics `json:"by_endpoint"`
}

// ModelStatistics contains per-model statistics.
type ModelStatistics struct {
	ModelName    string  `json:"model_name"`
	Requests     int64   `json:"requests"`
	Cost         float64 `json:"cost"`
	AvgLatency   float64 `json:"avg_latency"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

// EndpointStatistics contains per-endpoint statistics.
type EndpointStatistics struct {
	EndpointName string  `json:"endpoint_name"`
	Requests     int64   `json:"requests"`
	Cost         float64 `json:"cost"`
	AvgLatency   float64 `json:"avg_latency"`
	SuccessRate  float64 `json:"success_rate"`
}

// RoutingAggregation holds SQL-aggregated routing statistics.
type RoutingAggregation struct {
	TotalRequests   int64
	MethodCounts    map[string]int64
	RuleCounts      map[string]int64
	RuleIDs         map[string]*int64
	InaccurateCount int64
}

// EndpointModelStats contains historical per-endpoint-model statistics.
type EndpointModelStats struct {
	TotalRequests int64   `json:"total_requests"`
	TotalErrors   int64   `json:"total_errors"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}
