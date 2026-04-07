package service

import (
	"sync"

	"go.uber.org/zap"
)

// AsyncWorkerPool provides a bounded pool of goroutines for fire-and-forget
// background tasks (e.g. log inserts, counter updates). It replaces unbounded
// go func() calls that can accumulate goroutines under write-contention.
type AsyncWorkerPool struct {
	ch      chan func()
	wg      sync.WaitGroup
	logger  *zap.Logger
	once    sync.Once
	stopped chan struct{}
}

// NewAsyncWorkerPool creates a pool with the given number of workers and
// a buffered task queue of queueSize. If workers < 1 it defaults to 1.
func NewAsyncWorkerPool(workers int, queueSize int, logger *zap.Logger) *AsyncWorkerPool {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	p := &AsyncWorkerPool{
		ch:      make(chan func(), queueSize),
		logger:  logger,
		stopped: make(chan struct{}),
	}

	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	return p
}

// Submit enqueues a task for async execution. Returns false (and drops the
// task) if the queue is full or the pool has been shut down.
// If the pool is nil, falls back to an unbounded goroutine for backward
// compatibility with tests that pass nil.
func (p *AsyncWorkerPool) Submit(fn func()) bool {
	if p == nil {
		go fn()
		return true
	}

	// Two-phase select: first check if the pool is stopped, then try to enqueue.
	// If close(stopped) races between the two selects, the task lands in the
	// buffer while workers are still alive (Shutdown calls close(stopped) before
	// wg.Wait), so drain() in the workers will process it. This is safe.
	select {
	case <-p.stopped:
		return false
	default:
	}

	select {
	case p.ch <- fn:
		return true
	default:
		return false
	}
}

// Pending returns the number of tasks waiting in the queue.
func (p *AsyncWorkerPool) Pending() int {
	if p == nil {
		return 0
	}
	return len(p.ch)
}

// Shutdown signals workers to stop and waits for all in-flight tasks to finish.
// Safe to call multiple times. No-op if pool is nil.
func (p *AsyncWorkerPool) Shutdown() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		close(p.stopped)
	})
	p.wg.Wait()
}

func (p *AsyncWorkerPool) worker() {
	defer p.wg.Done()
	for {
		select {
		case fn, ok := <-p.ch:
			if !ok {
				// Channel was closed (should not happen in normal flow).
				return
			}
			p.safeRun(fn)
		case <-p.stopped:
			// Drain remaining tasks in the queue before exiting.
			p.drain()
			return
		}
	}
}

// drain processes any remaining tasks left in the channel buffer.
func (p *AsyncWorkerPool) drain() {
	for {
		select {
		case fn := <-p.ch:
			p.safeRun(fn)
		default:
			return
		}
	}
}

func (p *AsyncWorkerPool) safeRun(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("async task panicked", zap.Any("panic", r))
		}
	}()
	fn()
}
