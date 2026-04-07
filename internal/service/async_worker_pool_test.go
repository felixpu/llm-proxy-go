package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestAsyncWorkerPool_ExecutesTasks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(4, 16, logger)
	defer pool.Shutdown()

	var counter atomic.Int32

	for i := 0; i < 10; i++ {
		ok := pool.Submit(func() {
			counter.Add(1)
		})
		assert.True(t, ok, "submit should succeed")
	}

	// Wait for tasks to complete
	pool.Shutdown()
	assert.Equal(t, int32(10), counter.Load(), "all 10 tasks should execute")
}

func TestAsyncWorkerPool_RespectsWorkerCount(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(2, 100, logger)
	defer pool.Shutdown()

	var maxConcurrent atomic.Int32
	var current atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			cur := current.Add(1)
			// Track max concurrency
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
		})
	}

	wg.Wait()
	pool.Shutdown()

	// With 2 workers, max concurrency should be at most 2
	assert.LessOrEqual(t, maxConcurrent.Load(), int32(2),
		"max concurrent tasks should not exceed worker count")
	assert.GreaterOrEqual(t, maxConcurrent.Load(), int32(1),
		"at least 1 concurrent task should have run")
}

func TestAsyncWorkerPool_DropTaskWhenFull(t *testing.T) {
	logger := zaptest.NewLogger(t)
	// 1 worker, queue size 2 — very constrained
	pool := NewAsyncWorkerPool(1, 2, logger)
	defer pool.Shutdown()

	// Block the single worker
	blocker := make(chan struct{})
	pool.Submit(func() {
		<-blocker
	})

	// Fill the queue
	time.Sleep(5 * time.Millisecond) // Let worker pick up blocker task
	pool.Submit(func() {})           // slot 1
	pool.Submit(func() {})           // slot 2

	// This should be dropped
	ok := pool.Submit(func() {})
	assert.False(t, ok, "submit should return false when queue is full")

	close(blocker)
}

func TestAsyncWorkerPool_ShutdownWaitsForInFlight(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(2, 16, logger)

	var completed atomic.Int32

	for i := 0; i < 5; i++ {
		pool.Submit(func() {
			time.Sleep(20 * time.Millisecond)
			completed.Add(1)
		})
	}

	// Shutdown should block until all in-flight tasks finish
	pool.Shutdown()

	assert.Equal(t, int32(5), completed.Load(),
		"shutdown should wait for all queued tasks to complete")
}

func TestAsyncWorkerPool_ShutdownIsIdempotent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(2, 16, logger)

	pool.Submit(func() {})

	// Multiple shutdowns should not panic
	require.NotPanics(t, func() {
		pool.Shutdown()
		pool.Shutdown()
		pool.Shutdown()
	})
}

func TestAsyncWorkerPool_SubmitAfterShutdownReturnsFalse(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(2, 16, logger)
	pool.Shutdown()

	ok := pool.Submit(func() {})
	assert.False(t, ok, "submit after shutdown should return false")
}

func TestAsyncWorkerPool_PendingCount(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(1, 16, logger)
	defer pool.Shutdown()

	// Block the worker
	blocker := make(chan struct{})
	pool.Submit(func() {
		<-blocker
	})
	time.Sleep(5 * time.Millisecond) // let worker pick it up

	// Queue more tasks
	pool.Submit(func() {})
	pool.Submit(func() {})

	pending := pool.Pending()
	assert.GreaterOrEqual(t, pending, 2, "should report at least 2 pending tasks")

	close(blocker)
}

func TestAsyncWorkerPool_ZeroWorkersDefaultsToOne(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(0, 16, logger)
	defer pool.Shutdown()

	var executed atomic.Bool
	pool.Submit(func() {
		executed.Store(true)
	})

	pool.Shutdown()
	assert.True(t, executed.Load(), "task should still execute with default worker")
}

func TestAsyncWorkerPool_TaskPanicDoesNotCrashWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewAsyncWorkerPool(1, 16, logger)
	defer pool.Shutdown()

	var afterPanic atomic.Bool

	// Submit a panicking task
	pool.Submit(func() {
		panic("test panic")
	})

	// This task should still execute because the worker should recover
	time.Sleep(10 * time.Millisecond)
	pool.Submit(func() {
		afterPanic.Store(true)
	})

	pool.Shutdown()
	assert.True(t, afterPanic.Load(), "worker should recover from panic and continue processing")
}

func TestAsyncWorkerPool_NilPoolFallsBackToGoroutine(t *testing.T) {
	var pool *AsyncWorkerPool // nil

	var executed atomic.Bool
	ok := pool.Submit(func() {
		executed.Store(true)
	})
	assert.True(t, ok, "nil pool should accept submit")

	time.Sleep(20 * time.Millisecond)
	assert.True(t, executed.Load(), "task should execute via fallback goroutine")

	// Shutdown on nil should not panic
	require.NotPanics(t, func() {
		pool.Shutdown()
	})

	assert.Equal(t, 0, pool.Pending(), "nil pool pending should return 0")
}
