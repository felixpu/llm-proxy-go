package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// TestGetTask_ReturnsCopy verifies that GetTask returns a copy, not a pointer to internal state
func TestGetTask_ReturnsCopy(t *testing.T) {
	analyzer := &RoutingAnalyzer{
		tasks:  make(map[string]*models.AnalysisTask),
		logger: zap.NewNop(),
	}

	// Create a task
	taskID := "test-task-1"
	originalTask := &models.AnalysisTask{
		ID:       taskID,
		Status:   "pending",
		Progress: 10,
		Stage:    "initializing",
	}
	analyzer.tasks[taskID] = originalTask

	// Get the task
	retrieved := analyzer.GetTask(taskID)
	require.NotNil(t, retrieved)
	assert.Equal(t, "pending", retrieved.Status)
	assert.Equal(t, 10, retrieved.Progress)

	// Modify the retrieved copy
	retrieved.Status = "completed"
	retrieved.Progress = 100

	// Verify original task is unchanged
	assert.Equal(t, "pending", analyzer.tasks[taskID].Status)
	assert.Equal(t, 10, analyzer.tasks[taskID].Progress)
}

// TestGetTask_ConcurrentAccess verifies no race conditions when reading task state
func TestGetTask_ConcurrentAccess(t *testing.T) {
	analyzer := &RoutingAnalyzer{
		tasks:  make(map[string]*models.AnalysisTask),
		logger: zap.NewNop(),
	}

	taskID := "test-task-2"
	analyzer.tasks[taskID] = &models.AnalysisTask{
		ID:       taskID,
		Status:   "running",
		Progress: 50,
	}

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				task := analyzer.GetTask(taskID)
				if task != nil {
					_ = task.Status
					_ = task.Progress
				}
			}
		}()
	}

	// Concurrent writer (simulating updateTask)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < iterations; j++ {
			analyzer.updateTask(taskID, func(t *models.AnalysisTask) {
				t.Progress = j
			})
			time.Sleep(time.Microsecond)
		}
	}()

	wg.Wait()
}

// TestStartAnalysis_PreventsConcurrentAnalysis verifies TOCTOU race is fixed
func TestStartAnalysis_PreventsConcurrentAnalysis(t *testing.T) {
	// Mock repositories
	mockModelRepo := &mockRoutingModelRepository{
		model: &models.RoutingModelWithProvider{
			RoutingModel: models.RoutingModel{
				ID:         1,
				ProviderID: 1,
				ModelName:  "test-model",
				APIType:    "anthropic_messages",
			},
			APIKey:  "test-key",
			BaseURL: "https://api.anthropic.com/v1",
		},
	}

	analyzer := &RoutingAnalyzer{
		tasks:     make(map[string]*models.AnalysisTask),
		modelRepo: mockModelRepo,
		logger:    zap.NewNop(),
	}

	// Create a running task
	analyzer.tasks["existing-task"] = &models.AnalysisTask{
		ID:     "existing-task",
		Status: "running",
	}

	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()
	req := &models.AnalysisRequest{
		ModelID:   1,
		StartTime: &startTime,
		EndTime:   &endTime,
	}

	// Try to start another analysis - should fail
	_, err := analyzer.StartAnalysis(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "analysis already in progress")
}

// TestStartAnalysis_ConcurrentRequests verifies only one analysis starts under concurrent load
func TestStartAnalysis_ConcurrentRequests(t *testing.T) {
	mockModelRepo := &mockRoutingModelRepository{
		model: &models.RoutingModelWithProvider{
			RoutingModel: models.RoutingModel{
				ID:         1,
				ProviderID: 1,
				ModelName:  "test-model",
				APIType:    "anthropic_messages",
			},
			APIKey:  "test-key",
			BaseURL: "https://api.anthropic.com/v1",
		},
	}

	mockLogRepo := &mockRequestLogRepository{}

	analyzer := &RoutingAnalyzer{
		tasks:     make(map[string]*models.AnalysisTask),
		modelRepo: mockModelRepo,
		logRepo:   mockLogRepo,
		logger:    zap.NewNop(),
	}

	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()
	req := &models.AnalysisRequest{
		ModelID:   1,
		StartTime: &startTime,
		EndTime:   &endTime,
	}

	var wg sync.WaitGroup
	var successIDs []string
	var mu sync.Mutex

	// Launch 10 concurrent StartAnalysis calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			taskID, err := analyzer.StartAnalysis(context.Background(), req)
			mu.Lock()
			if err == nil {
				successIDs = append(successIDs, taskID)
				t.Logf("Goroutine %d: SUCCESS - taskID=%s", idx, taskID)
			} else {
				t.Logf("Goroutine %d: FAILED - %v", idx, err)
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Give goroutines a moment to start (they will fail on nil dependencies, but that's ok)
	time.Sleep(10 * time.Millisecond)

	// Only one should succeed
	t.Logf("Total successful starts: %d, IDs: %v", len(successIDs), successIDs)
	assert.Equal(t, 1, len(successIDs), "exactly one analysis should start")
}

// Mock repository for testing
type mockRoutingModelRepository struct {
	model *models.RoutingModelWithProvider
	err   error
}

func (m *mockRoutingModelRepository) GetModelWithProviderAny(ctx context.Context, modelID int64) (*models.RoutingModelWithProvider, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.model, nil
}

// Mock log repository - embed interface and override only what we need
type mockRequestLogRepository struct {
	// Embed a nil interface pointer - methods will panic if called, but we only need CountForAnalysis
}

func (m *mockRequestLogRepository) Insert(ctx context.Context, entry *models.RequestLogEntry) (int64, error) {
	return 0, nil
}

func (m *mockRequestLogRepository) GetByID(ctx context.Context, id int64) (*models.RequestLog, error) {
	return nil, nil
}

func (m *mockRequestLogRepository) List(ctx context.Context, limit, offset int, userID *int64, modelName, endpointName *string, startTime, endTime *time.Time, success *bool) ([]*models.RequestLog, int64, error) {
	return nil, 0, nil
}

func (m *mockRequestLogRepository) GetStatistics(ctx context.Context, startTime, endTime *time.Time, userID *int64, modelName, endpointName *string, success *bool) (*repository.LogStatistics, error) {
	return nil, nil
}

func (m *mockRequestLogRepository) Count(ctx context.Context, modelName, endpointName *string, startTime, endTime *time.Time) (int64, error) {
	return 0, nil
}

func (m *mockRequestLogRepository) Delete(ctx context.Context, modelName, endpointName *string, startTime, endTime *time.Time) (int64, error) {
	return 0, nil
}

func (m *mockRequestLogRepository) MarkInaccurate(ctx context.Context, id int64, inaccurate bool) error {
	return nil
}

func (m *mockRequestLogRepository) GetRoutingAggregation(ctx context.Context, startTime, endTime *time.Time) (*repository.RoutingAggregation, error) {
	return nil, nil
}

func (m *mockRequestLogRepository) ListInaccurate(ctx context.Context, limit, offset int) ([]*models.RequestLog, int64, error) {
	return nil, 0, nil
}

func (m *mockRequestLogRepository) ListForAnalysis(ctx context.Context, startTime, endTime *time.Time, maxResults int) ([]*models.RequestLog, error) {
	return nil, nil
}

func (m *mockRequestLogRepository) CountForAnalysis(ctx context.Context, startTime, endTime *time.Time) (int, error) {
	// Block to keep the goroutine running during the test
	time.Sleep(100 * time.Millisecond)
	return 0, nil
}

func (m *mockRequestLogRepository) GetEndpointModelStats(ctx context.Context) (map[string]*repository.EndpointModelStats, error) {
	return nil, nil
}
