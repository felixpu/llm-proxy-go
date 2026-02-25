package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

const (
	// HeartbeatInterval is the interval between heartbeat updates
	HeartbeatInterval = 10 * time.Second
	// HeartbeatTimeout is the timeout for considering a worker stale
	HeartbeatTimeout = 30 * time.Second
)

// WorkerCoordinator manages worker registration and primary election
type WorkerCoordinator struct {
	workerID   string
	pid        int
	isPrimary  bool
	running    bool
	workerRepo *repository.WorkerRegistryRepository
	stateRepo  *repository.SharedStateRepository
	logger     *zap.Logger

	mu       sync.RWMutex
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewWorkerCoordinator creates a new WorkerCoordinator
func NewWorkerCoordinator(db *sql.DB, logger *zap.Logger) *WorkerCoordinator {
	return &WorkerCoordinator{
		workerID:   uuid.New().String(),
		pid:        os.Getpid(),
		isPrimary:  false,
		running:    false,
		workerRepo: repository.NewWorkerRegistryRepository(db, logger),
		stateRepo:  repository.NewSharedStateRepository(db, logger),
		logger:     logger,
		done:       make(chan struct{}),
	}
}

// Register registers this worker and attempts to become primary
func (wc *WorkerCoordinator) Register(ctx context.Context) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	// Cleanup stale workers first
	_, err := wc.workerRepo.CleanupStale(ctx, int(HeartbeatTimeout.Seconds()))
	if err != nil {
		wc.logger.Warn("failed to cleanup stale workers", zap.Error(err))
	}

	// Register this worker
	_, err = wc.workerRepo.Register(ctx, wc.workerID, wc.pid)
	if err != nil {
		return err
	}

	// Try to become primary
	isPrimary, err := wc.workerRepo.TryBecomePrimary(ctx, wc.workerID)
	if err != nil {
		wc.logger.Warn("failed to try become primary", zap.Error(err))
	}
	wc.isPrimary = isPrimary

	wc.logger.Info("worker registered",
		zap.String("worker_id", wc.workerID),
		zap.Int("pid", wc.pid),
		zap.Bool("is_primary", wc.isPrimary))

	return nil
}

// Start begins the heartbeat loop
func (wc *WorkerCoordinator) Start(ctx context.Context) {
	wc.mu.Lock()
	if wc.running {
		wc.mu.Unlock()
		return
	}
	wc.running = true
	wc.done = make(chan struct{})
	wc.mu.Unlock()

	wc.wg.Add(1)
	go wc.heartbeatLoop(ctx)

	wc.logger.Info("worker coordinator started",
		zap.String("worker_id", wc.workerID),
		zap.Duration("heartbeat_interval", HeartbeatInterval))
}

// heartbeatLoop periodically updates heartbeat and checks for primary failover
func (wc *WorkerCoordinator) heartbeatLoop(ctx context.Context) {
	defer wc.wg.Done()

	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-wc.done:
			return
		case <-ticker.C:
			wc.doHeartbeat(ctx)
		}
	}
}

// doHeartbeat performs a single heartbeat update and checks for failover
func (wc *WorkerCoordinator) doHeartbeat(ctx context.Context) {
	// Update heartbeat
	if err := wc.workerRepo.UpdateHeartbeat(ctx, wc.workerID); err != nil {
		wc.logger.Error("failed to update heartbeat", zap.Error(err))
		return
	}

	wc.mu.RLock()
	isPrimary := wc.isPrimary
	wc.mu.RUnlock()

	// If not primary, check if we should try to become primary
	if !isPrimary {
		stale, err := wc.workerRepo.IsPrimaryStale(ctx, int(HeartbeatTimeout.Seconds()))
		if err != nil {
			wc.logger.Error("failed to check primary staleness", zap.Error(err))
			return
		}

		if stale {
			// Cleanup stale workers (including old primary)
			_, _ = wc.workerRepo.CleanupStale(ctx, int(HeartbeatTimeout.Seconds()))

			// Try to become primary
			became, err := wc.workerRepo.TryBecomePrimary(ctx, wc.workerID)
			if err != nil {
				wc.logger.Error("failed to try become primary", zap.Error(err))
				return
			}

			if became {
				wc.mu.Lock()
				wc.isPrimary = true
				wc.mu.Unlock()

				wc.logger.Info("worker became primary after failover",
					zap.String("worker_id", wc.workerID))
			}
		}
	}
}

// IsPrimary returns whether this worker is the primary
func (wc *WorkerCoordinator) IsPrimary() bool {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	return wc.isPrimary
}

// WorkerID returns the unique ID of this worker
func (wc *WorkerCoordinator) WorkerID() string {
	return wc.workerID
}

// PID returns the process ID of this worker
func (wc *WorkerCoordinator) PID() int {
	return wc.pid
}

// SetSharedState sets a shared state value (JSON serialized)
func (wc *WorkerCoordinator) SetSharedState(ctx context.Context, key string, value any) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return wc.stateRepo.SetState(ctx, key, string(jsonValue), wc.workerID)
}

// GetSharedState retrieves a shared state value (JSON deserialized)
func (wc *WorkerCoordinator) GetSharedState(ctx context.Context, key string, dest any) error {
	state, err := wc.stateRepo.GetState(ctx, key)
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}
	return json.Unmarshal([]byte(state.Value), dest)
}

// Unregister removes this worker from the registry
func (wc *WorkerCoordinator) Unregister(ctx context.Context) error {
	wc.mu.Lock()
	wc.running = false
	if wc.done != nil {
		close(wc.done)
	}
	wc.mu.Unlock()

	// Wait for heartbeat loop to finish
	wc.wg.Wait()

	err := wc.workerRepo.Unregister(ctx, wc.workerID)
	if err != nil {
		return err
	}

	wc.logger.Info("worker unregistered", zap.String("worker_id", wc.workerID))
	return nil
}

// Stop stops the coordinator without unregistering
func (wc *WorkerCoordinator) Stop() {
	wc.mu.Lock()
	if !wc.running {
		wc.mu.Unlock()
		return
	}
	wc.running = false
	close(wc.done)
	wc.mu.Unlock()

	wc.wg.Wait()
	wc.logger.Info("worker coordinator stopped", zap.String("worker_id", wc.workerID))
}
