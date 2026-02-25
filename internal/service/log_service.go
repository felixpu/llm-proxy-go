package service

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// LogService handles request logging with async batch writes
type LogService struct {
	repo          *repository.RequestLogRepositoryImpl
	logger        *zap.Logger
	logChan       chan *models.RequestLogEntry
	done          chan struct{}
	wg            sync.WaitGroup
	batchSize     int
	flushInterval time.Duration
}

// NewLogService creates a new LogService
func NewLogService(db *sql.DB, logger *zap.Logger) *LogService {
	ls := &LogService{
		repo:          repository.NewRequestLogRepositoryImpl(db, logger),
		logger:        logger,
		logChan:       make(chan *models.RequestLogEntry, 1000),
		done:          make(chan struct{}),
		batchSize:     100,
		flushInterval: 5 * time.Second,
	}

	ls.wg.Add(1)
	go ls.batchWriter()

	return ls
}

// LogRequest queues a request log entry for async writing
func (ls *LogService) LogRequest(entry *models.RequestLogEntry) {
	select {
	case ls.logChan <- entry:
	default:
		ls.logger.Warn("log channel full, writing synchronously")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := ls.repo.Insert(ctx, entry); err != nil {
			ls.logger.Error("failed to insert log", zap.Error(err))
		}
	}
}

// batchWriter processes log entries in batches
func (ls *LogService) batchWriter() {
	defer ls.wg.Done()

	batch := make([]*models.RequestLogEntry, 0, ls.batchSize)
	ticker := time.NewTicker(ls.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, entry := range batch {
			if _, err := ls.repo.Insert(ctx, entry); err != nil {
				ls.logger.Error("failed to insert log entry", zap.Error(err))
			}
		}
		ls.logger.Debug("flushed log batch", zap.Int("count", len(batch)))
		batch = batch[:0]
	}

	for {
		select {
		case <-ls.done:
			flush()
			return
		case entry := <-ls.logChan:
			batch = append(batch, entry)
			if len(batch) >= ls.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Stop gracefully stops the log service
func (ls *LogService) Stop() {
	close(ls.done)
	ls.wg.Wait()
	ls.logger.Info("log service stopped")
}

// ListLogs retrieves request logs with filtering and pagination
func (ls *LogService) ListLogs(ctx context.Context, filters *LogFilters) ([]*models.RequestLog, int64, error) {
	return ls.repo.List(ctx, filters.Limit, filters.Offset, filters.UserID, filters.ModelName,
		filters.EndpointName, filters.StartTime, filters.EndTime, filters.Success)
}

// GetStatistics retrieves aggregated statistics
func (ls *LogService) GetStatistics(ctx context.Context, filters *LogFilters) (*repository.LogStatistics, error) {
	return ls.repo.GetStatistics(ctx, filters.StartTime, filters.EndTime, filters.UserID,
		filters.ModelName, filters.EndpointName, filters.Success)
}

// CountLogs counts logs matching the filters
func (ls *LogService) CountLogs(ctx context.Context, filters *LogFilters) (int64, error) {
	return ls.repo.Count(ctx, filters.ModelName, filters.EndpointName, filters.StartTime, filters.EndTime)
}

// DeleteLogs deletes logs matching the filters
func (ls *LogService) DeleteLogs(ctx context.Context, filters *LogFilters) (int64, error) {
	return ls.repo.Delete(ctx, filters.ModelName, filters.EndpointName, filters.StartTime, filters.EndTime)
}

// LogFilters contains filter parameters for log queries
type LogFilters struct {
	Limit        int
	Offset       int
	UserID       *int64
	ModelName    *string
	EndpointName *string
	StartTime    *time.Time
	EndTime      *time.Time
	Success      *bool
}

// CalculateCost calculates the cost for a request based on token usage
func CalculateCost(model *models.Model, inputTokens, outputTokens int) float64 {
	inputCost := float64(inputTokens) / 1_000_000 * model.CostPerMtokInput
	outputCost := float64(outputTokens) / 1_000_000 * model.CostPerMtokOutput * model.BillingMultiplier
	return logRoundTo(inputCost+outputCost, 6)
}

func logRoundTo(val float64, places int) float64 {
	mult := math.Pow(10, float64(places))
	return math.Round(val*mult) / mult
}
