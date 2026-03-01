package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// RoutingAnalyzer performs LLM-based routing rule analysis.
type RoutingAnalyzer struct {
	logRepo    repository.RequestLogRepository
	ruleRepo   repository.RoutingRuleRepository
	modelRepo  *repository.RoutingModelRepository
	reportRepo *repository.AnalysisReportRepository
	extractor  *MessageExtractor
	logger     *zap.Logger
	client     *http.Client

	mu    sync.RWMutex
	tasks map[string]*models.AnalysisTask
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
	// Limit concurrency: only 1 analysis at a time
	a.mu.RLock()
	for _, t := range a.tasks {
		if t.Status == "pending" || t.Status == "running" {
			a.mu.RUnlock()
			return "", fmt.Errorf("analysis already in progress (task %s)", t.ID)
		}
	}
	a.mu.RUnlock()

	// Validate model (any status, user explicitly chose it)
	modelCfg, err := a.modelRepo.GetModelWithProviderAny(ctx, req.ModelID)
	if err != nil {
		return "", fmt.Errorf("failed to load model %d: %w", req.ModelID, err)
	}
	if modelCfg == nil {
		return "", fmt.Errorf("model_id %d not found or provider missing", req.ModelID)
	}

	taskID := fmt.Sprintf("analysis-%d", time.Now().UnixMilli())
	task := &models.AnalysisTask{
		ID:        taskID,
		Status:    "pending",
		Progress:  0,
		Stage:     "initializing",
		CreatedAt: time.Now(),
	}

	a.mu.Lock()
	a.tasks[taskID] = task
	a.mu.Unlock()

	go a.runAnalysis(taskID, req, modelCfg)
	return taskID, nil
}

// GetTask returns the current state of an analysis task.
func (a *RoutingAnalyzer) GetTask(taskID string) *models.AnalysisTask {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.tasks[taskID]
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
		t.Status = "running"
		t.Stage = "collecting_logs"
		t.Progress = 5
	})

	// Step 1: Collect logs
	maxResults := 500
	logs, err := a.logRepo.ListForAnalysis(ctx, req.StartTime, req.EndTime, maxResults)
	if err != nil {
		a.failTask(taskID, fmt.Sprintf("collect logs: %v", err))
		return
	}
	totalLogs := len(logs)
	if totalLogs == 0 {
		a.failTask(taskID, "no logs found in the specified time range")
		return
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "extracting_messages"
		t.Progress = 15
	})

	// Step 2: Smart sampling — keep all if <=200, otherwise sample
	sampled := a.sampleLogs(logs, 200)

	// Step 3: Extract messages
	entries := make([]*models.ExtractedLogEntry, 0, len(sampled))
	for _, log := range sampled {
		entries = append(entries, a.extractor.ExtractFromLog(log))
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "loading_rules"
		t.Progress = 25
	})

	// Step 4: Load rules
	rules, err := a.ruleRepo.ListRules(ctx, false)
	if err != nil {
		a.failTask(taskID, fmt.Sprintf("load rules: %v", err))
		return
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "building_prompt"
		t.Progress = 35
	})

	// Step 5: Build prompt
	userPrompt := BuildAnalysisPrompt(rules, entries, totalLogs, len(entries))

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "calling_llm"
		t.Progress = 45
	})

	// Step 6: Call LLM
	llmResponse, err := a.callAnalysisModel(ctx, userPrompt, modelCfg)
	if err != nil {
		a.failTask(taskID, fmt.Sprintf("LLM call: %v", err))
		return
	}

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "parsing_result"
		t.Progress = 85
	})

	// Step 7: Parse response
	report, err := ParseAnalysisResponse(llmResponse)
	if err != nil {
		a.failTask(taskID, fmt.Sprintf("parse response: %v", err))
		return
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

// callAnalysisModel calls the LLM via OpenAI-compatible chat API.
func (a *RoutingAnalyzer) callAnalysisModel(ctx context.Context, userPrompt string, modelCfg *models.RoutingModelWithProvider) (string, error) {
	reqBody := map[string]any{
		"model":       modelCfg.ModelName,
		"max_tokens":  4096,
		"temperature": 0.1,
		"messages": []map[string]string{
			{"role": "system", "content": AnalysisSystemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", modelCfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+modelCfg.APIKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from model")
	}

	return chatResp.Choices[0].Message.Content, nil
}
