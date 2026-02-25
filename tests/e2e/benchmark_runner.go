//go:build e2e
// +build e2e

package e2e_test

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// BenchmarkResult represents the result of a performance benchmark.
type BenchmarkResult struct {
	Name           string
	TotalRequests  int64
	SuccessCount   int64
	ErrorCount     int64
	Duration       time.Duration
	QPS            float64
	AvgLatency     time.Duration
	P50Latency     time.Duration
	P99Latency     time.Duration
	MaxLatency     time.Duration
	MinLatency     time.Duration
	MemoryUsed     uint64
	MemoryPeakUsed uint64
}

// PerformanceBenchmark runs performance benchmarks.
type PerformanceBenchmark struct {
	name           string
	concurrency    int
	duration       time.Duration
	requestFunc    func() error
	latencies      []time.Duration
	mu             sync.Mutex
}

// NewPerformanceBenchmark creates a new performance benchmark.
func NewPerformanceBenchmark(name string, concurrency int, duration time.Duration, requestFunc func() error) *PerformanceBenchmark {
	return &PerformanceBenchmark{
		name:        name,
		concurrency: concurrency,
		duration:    duration,
		requestFunc: requestFunc,
		latencies:   make([]time.Duration, 0),
	}
}

// Run executes the benchmark.
func (pb *PerformanceBenchmark) Run() *BenchmarkResult {
	var (
		totalRequests int64
		successCount  int64
		errorCount    int64
		wg            sync.WaitGroup
		done          = make(chan struct{})
		startTime     = time.Now()
	)

	// Record initial memory
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	initialMemory := m1.Alloc

	// Start workers
	for i := 0; i < pb.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					reqStart := time.Now()
					err := pb.requestFunc()
					latency := time.Since(reqStart)

					atomic.AddInt64(&totalRequests, 1)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
					} else {
						atomic.AddInt64(&successCount, 1)
					}

					pb.mu.Lock()
					pb.latencies = append(pb.latencies, latency)
					pb.mu.Unlock()
				}
			}
		}()
	}

	// Wait for duration
	time.Sleep(pb.duration)
	close(done)
	wg.Wait()

	// Record final memory
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	peakMemory := m2.Alloc

	// Calculate results
	elapsed := time.Since(startTime)
	qps := float64(atomic.LoadInt64(&totalRequests)) / elapsed.Seconds()

	// Calculate latency percentiles
	p50, p99, avgLatency, minLatency, maxLatency := pb.calculateLatencies()

	return &BenchmarkResult{
		Name:           pb.name,
		TotalRequests:  atomic.LoadInt64(&totalRequests),
		SuccessCount:   atomic.LoadInt64(&successCount),
		ErrorCount:     atomic.LoadInt64(&errorCount),
		Duration:       elapsed,
		QPS:            qps,
		AvgLatency:     avgLatency,
		P50Latency:     p50,
		P99Latency:     p99,
		MaxLatency:     maxLatency,
		MinLatency:     minLatency,
		MemoryUsed:     peakMemory - initialMemory,
		MemoryPeakUsed: peakMemory,
	}
}

// calculateLatencies calculates latency percentiles.
func (pb *PerformanceBenchmark) calculateLatencies() (p50, p99, avg, min, max time.Duration) {
	if len(pb.latencies) == 0 {
		return 0, 0, 0, 0, 0
	}

	// Sort latencies (simple bubble sort for small datasets)
	latencies := make([]time.Duration, len(pb.latencies))
	copy(latencies, pb.latencies)
	for i := 0; i < len(latencies); i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[j] < latencies[i] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	// Calculate percentiles
	min = latencies[0]
	max = latencies[len(latencies)-1]

	p50Index := len(latencies) / 2
	p50 = latencies[p50Index]

	p99Index := (len(latencies) * 99) / 100
	if p99Index >= len(latencies) {
		p99Index = len(latencies) - 1
	}
	p99 = latencies[p99Index]

	// Calculate average
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	avg = sum / time.Duration(len(latencies))

	return
}

// String returns a formatted string representation of the benchmark result.
func (br *BenchmarkResult) String() string {
	return fmt.Sprintf(`
Benchmark: %s
Duration: %v
Total Requests: %d
Success: %d
Errors: %d
QPS: %.2f
Avg Latency: %v
P50 Latency: %v
P99 Latency: %v
Min Latency: %v
Max Latency: %v
Memory Used: %d bytes
Memory Peak: %d bytes
`, br.Name, br.Duration, br.TotalRequests, br.SuccessCount, br.ErrorCount,
		br.QPS, br.AvgLatency, br.P50Latency, br.P99Latency, br.MinLatency, br.MaxLatency,
		br.MemoryUsed, br.MemoryPeakUsed)
}

// ComparisonResult represents a comparison between two benchmark results.
type ComparisonResult struct {
	Python *BenchmarkResult
	Go     *BenchmarkResult
	QPSImprovement float64
	LatencyImprovement float64
	MemoryImprovement float64
}

// Compare compares two benchmark results.
func Compare(pythonResult, goResult *BenchmarkResult) *ComparisonResult {
	qpsImprovement := (goResult.QPS - pythonResult.QPS) / pythonResult.QPS * 100
	latencyImprovement := (pythonResult.AvgLatency.Seconds() - goResult.AvgLatency.Seconds()) / pythonResult.AvgLatency.Seconds() * 100
	memoryImprovement := (float64(pythonResult.MemoryUsed) - float64(goResult.MemoryUsed)) / float64(pythonResult.MemoryUsed) * 100

	return &ComparisonResult{
		Python:             pythonResult,
		Go:                 goResult,
		QPSImprovement:     qpsImprovement,
		LatencyImprovement: latencyImprovement,
		MemoryImprovement: memoryImprovement,
	}
}

// String returns a formatted string representation of the comparison result.
func (cr *ComparisonResult) String() string {
	return fmt.Sprintf(`
Performance Comparison:
QPS Improvement: %.2f%% (Python: %.2f, Go: %.2f)
Latency Improvement: %.2f%% (Python: %v, Go: %v)
Memory Improvement: %.2f%% (Python: %d bytes, Go: %d bytes)
`, cr.QPSImprovement, cr.Python.QPS, cr.Go.QPS,
		cr.LatencyImprovement, cr.Python.AvgLatency, cr.Go.AvgLatency,
		cr.MemoryImprovement, cr.Python.MemoryUsed, cr.Go.MemoryUsed)
}
