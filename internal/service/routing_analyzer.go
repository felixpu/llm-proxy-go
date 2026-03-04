package service

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// RoutingModelRepositoryInterface defines the interface for routing model repository operations.
type RoutingModelRepositoryInterface interface {
	GetModelWithProviderAny(ctx context.Context, modelID int64) (*models.RoutingModelWithProvider, error)
}

// RoutingAnalyzer performs LLM-based routing rule analysis.
type RoutingAnalyzer struct {
	logRepo    repository.RequestLogRepository
	ruleRepo   repository.RoutingRuleRepository
	modelRepo  RoutingModelRepositoryInterface
	reportRepo *repository.AnalysisReportRepository
	extractor  *MessageExtractor
	logger     *zap.Logger
	client     *http.Client

	mu       sync.RWMutex
	tasks    map[string]*models.AnalysisTask
	idCounter int64 // Atomic counter for unique task IDs
}

// NewRoutingAnalyzer creates a new RoutingAnalyzer.
func NewRoutingAnalyzer(
	logRepo repository.RequestLogRepository,
	ruleRepo repository.RoutingRuleRepository,
	modelRepo *repository.RoutingModelRepository,
	reportRepo *repository.AnalysisReportRepository,
	logger *zap.Logger,
) *RoutingAnalyzer {
	return &RoutingAnalyzer{
		logRepo:    logRepo,
		ruleRepo:   ruleRepo,
		modelRepo:  modelRepo,
		reportRepo: reportRepo,
		extractor:  &MessageExtractor{},
		logger:     logger,
		client:     &http.Client{Timeout: 120 * time.Second},
		tasks:      make(map[string]*models.AnalysisTask),
	}
}

// StartAnalysis launches an async analysis task and returns its ID.
func (a *RoutingAnalyzer) StartAnalysis(ctx context.Context, req *models.AnalysisRequest) (string, error) {
	// Validate model (any status, user explicitly chose it)
	modelCfg, err := a.modelRepo.GetModelWithProviderAny(ctx, req.ModelID)
	if err != nil {
		return "", fmt.Errorf("failed to load model %d: %w", req.ModelID, err)
	}
	if modelCfg == nil {
		return "", fmt.Errorf("model_id %d not found or provider missing", req.ModelID)
	}

	// Validate model configuration
	if err := validateModelConfig(modelCfg); err != nil {
		return "", fmt.Errorf("invalid model configuration: %w", err)
	}

	// Use single write lock for check-and-insert to prevent TOCTOU race
	a.mu.Lock()
	defer a.mu.Unlock()

	// Limit concurrency: only 1 analysis at a time
	for _, t := range a.tasks {
		if t.Status == "pending" || t.Status == "running" {
			return "", fmt.Errorf("analysis already in progress (task %s)", t.ID)
		}
	}

	taskID := fmt.Sprintf("analysis-%d-%d", time.Now().UnixMilli(), atomic.AddInt64(&a.idCounter, 1))
	task := &models.AnalysisTask{
		ID:        taskID,
		Status:    "running", // Set to running immediately to prevent race
		Progress:  0,
		Stage:     "initializing",
		CreatedAt: time.Now(),
	}

	a.tasks[taskID] = task

	go a.runAnalysis(taskID, req, modelCfg)
	return taskID, nil
}

// GetTask returns a copy of the current state of an analysis task.
// Returns nil if task not found.
func (a *RoutingAnalyzer) GetTask(taskID string) *models.AnalysisTask {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if t, ok := a.tasks[taskID]; ok {
		// Return a copy to prevent race conditions
		copy := *t
		return &copy
	}
	return nil
}

func (a *RoutingAnalyzer) updateTask(taskID string, fn func(t *models.AnalysisTask)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if t, ok := a.tasks[taskID]; ok {
		fn(t)
	}
}

func (a *RoutingAnalyzer) runAnalysis(taskID string, req *models.AnalysisRequest, modelCfg *models.RoutingModelWithProvider) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "collecting_logs"
		t.Progress = 5
	})

	// Step 1: Count total logs in time range
	actualTotal, err := a.logRepo.CountForAnalysis(ctx, req.StartTime, req.EndTime)
	if err != nil {
		a.failTask(taskID, fmt.Sprintf("count logs: %v", err))
		return
	}
	if actualTotal == 0 {
		a.failTask(taskID, "no logs found in the specified time range")
		return
	}

	// Step 2: Collect logs with intelligent limit based on total count
	config := DefaultAnalysisConfig()
	maxResults := config.MaxLogsPerBatch
	if actualTotal > 1000 {
		// For large datasets, collect more logs for better sampling
		maxResults = 1000
	}
	logs, err := a.logRepo.ListForAnalysis(ctx, req.StartTime, req.EndTime, maxResults)
	if err != nil {
		a.failTask(taskID, fmt.Sprintf("collect logs: %v", err))
		return
	}
	totalLogs := actualTotal

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "extracting_messages"
		t.Progress = 15
	})

	// Step 3: Smart sampling — prioritize inaccurate logs
	sampled := a.sampleLogs(logs, config.MaxLogsPerBatch)

	// Step 4: Extract messages
	entries := make([]*models.ExtractedLogEntry, 0, len(sampled))
	for _, log := range sampled {
		entries = append(entries, a.extractor.ExtractFromLog(log))
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "loading_rules"
		t.Progress = 25
	})

	// Step 5: Load rules
	rules, err := a.ruleRepo.ListRules(ctx, false)
	if err != nil {
		a.failTask(taskID, fmt.Sprintf("load rules: %v", err))
		return
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "analyzing"
		t.Progress = 35
	})

	// Step 6: Choose analysis strategy based on data volume
	var report *models.AnalysisReport
	if len(entries) <= 200 {
		// Single-pass analysis for small datasets
		report, err = a.runSinglePassAnalysis(ctx, taskID, rules, entries, totalLogs, modelCfg, config)
	} else {
		// Batched analysis for large datasets
		report, err = a.runBatchedAnalysis(ctx, taskID, rules, entries, totalLogs, modelCfg, config)
	}

	if err != nil {
		a.failTask(taskID, fmt.Sprintf("analysis failed: %v", err))
		return
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "parsing_result"
		t.Progress = 85
	})

	if report.Summary == nil && len(report.Issues) == 0 && report.Conclusion == "" {
		a.logger.Warn("analysis report has empty content, LLM response may not match expected format",
			zap.String("task_id", taskID),
			zap.Int("analyzed_logs", len(entries)),
		)
	}

	report.ModelUsed = modelCfg.ModelName
	report.TimeRangeStart = req.StartTime
	report.TimeRangeEnd = req.EndTime
	report.TotalLogs = totalLogs
	report.AnalyzedLogs = len(entries)

	// Step 8: Persist report
	reportID, err := a.reportRepo.Save(ctx, report)
	if err != nil {
		a.logger.Warn("failed to persist analysis report", zap.Error(err))
	} else {
		report.ID = reportID
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Status = "completed"
		t.Stage = "done"
		t.Progress = 100
		t.Report = report
	})

	a.logger.Info("routing analysis completed",
		zap.String("task_id", taskID),
		zap.Int("total_logs", totalLogs),
		zap.Int("analyzed", len(entries)),
		zap.Int("issues", len(report.Issues)),
		zap.Int("recommendations", len(report.Recommendations)),
	)
}

func (a *RoutingAnalyzer) failTask(taskID, errMsg string) {
	a.logger.Error("routing analysis failed", zap.String("task_id", taskID), zap.String("error", errMsg))
	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Status = "failed"
		t.Error = errMsg
	})
}

// sampleLogs picks up to maxSamples logs, prioritizing inaccurate ones.
func (a *RoutingAnalyzer) sampleLogs(logs []*models.RequestLog, maxSamples int) []*models.RequestLog {
	if len(logs) <= maxSamples {
		return logs
	}

	// Separate inaccurate and normal logs (logs are already ordered by is_inaccurate DESC)
	var inaccurate, normal []*models.RequestLog
	for _, l := range logs {
		if l.IsInaccurate {
			inaccurate = append(inaccurate, l)
		} else {
			normal = append(normal, l)
		}
	}

	// Include all inaccurate logs first
	result := make([]*models.RequestLog, 0, maxSamples)
	result = append(result, inaccurate...)
	if len(result) >= maxSamples {
		return result[:maxSamples]
	}

	// Fill remaining with evenly spaced normal logs
	remaining := maxSamples - len(result)
	if remaining > 0 && len(normal) > 0 {
		step := float64(len(normal)) / float64(remaining)
		for i := 0; i < remaining && int(float64(i)*step) < len(normal); i++ {
			result = append(result, normal[int(float64(i)*step)])
		}
	}

	return result
}

// validateModelConfig validates that the model configuration has all required fields.
func validateModelConfig(cfg *models.RoutingModelWithProvider) error {
	if cfg.ModelName == "" {
		return fmt.Errorf("model_name is empty")
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("base_url is empty")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("api_key is empty")
	}

	// Validate api_type
	validTypes := []string{"anthropic_messages", "anthropic_responses", "openai_chat"}
	apiType := cfg.APIType
	if apiType == "" {
		apiType = cfg.ProviderAPIType
	}
	if apiType == "" || apiType == "auto" {
		return fmt.Errorf("api_type must be explicitly set (not 'auto') for analysis. Supported types: %v", validTypes)
	}

	isValid := false
	for _, t := range validTypes {
		if apiType == t {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("unsupported api_type '%s'. Supported types: %v", apiType, validTypes)
	}

	return nil
}

// callAnalysisModel calls the LLM via the appropriate API adapter.
func (a *RoutingAnalyzer) callAnalysisModel(ctx context.Context, userPrompt string, modelCfg *models.RoutingModelWithProvider) (string, error) {
	return CallLLMModel(ctx, LLMCallParams{
		ModelCfg: modelCfg,
		Messages: []Message{
			{Role: "system", Content: AnalysisSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Options: RequestOptions{
			Model:       modelCfg.ModelName,
			MaxTokens:   8192,
			Temperature: 0.1,
			Stream:      false,
		},
		Client:     a.client,
		Logger:     a.logger,
		LogContext:  "analysis",
	})
}
