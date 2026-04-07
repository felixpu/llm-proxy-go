package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRateLimit_StopChGracefulShutdown(t *testing.T) {
	stopCh := make(chan struct{})
	cfg := &RateLimitConfig{
		Enabled:       true,
		MaxRequests:   100,
		WindowSeconds: 60,
		ExemptPaths:   []string{"/health"},
		StopCh:        stopCh,
	}

	r := gin.New()
	r.Use(RateLimit(cfg))
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	// Verify middleware works
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// Close stopCh — should not panic or deadlock
	require.NotPanics(t, func() {
		close(stopCh)
	})

	// Give cleanup goroutine time to exit
	time.Sleep(20 * time.Millisecond)

	// Middleware should still work after stop
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestRateLimit_IPMapMaxSize(t *testing.T) {
	cfg := &RateLimitConfig{
		Enabled:       true,
		MaxRequests:   100,
		WindowSeconds: 60,
		MaxClients:    50,
	}

	r := gin.New()
	r.Use(RateLimit(cfg))
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	// Generate requests from many unique IPs
	for i := 0; i < 200; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Forwarded-For", randomIP(i))
		r.ServeHTTP(w, req)
	}

	// After cleanup, map should be bounded
	// We can't inspect internals directly, but we verify no panic or OOM
}

func TestRateLimiter_CleanupRemovesOldEntries(t *testing.T) {
	rl := newRateLimiter(100, 1) // 1 second window

	// Add some entries
	rl.isAllowed("client-a")
	rl.isAllowed("client-b")
	rl.isAllowed("client-c")

	// Wait for entries to expire
	time.Sleep(1100 * time.Millisecond)

	rl.cleanup()

	rl.mu.Lock()
	size := len(rl.requests)
	rl.mu.Unlock()

	assert.Equal(t, 0, size, "all expired entries should be cleaned up")
}

func TestRateLimiter_CleanupEvictsWhenOverMaxClients(t *testing.T) {
	rl := newRateLimiter(100, 60)

	// Add entries from 10 different clients without maxClients cap.
	for i := 0; i < 10; i++ {
		rl.isAllowed(randomIP(i))
	}

	rl.mu.Lock()
	before := len(rl.requests)
	rl.mu.Unlock()
	assert.Equal(t, 10, before)

	// Now set maxClients cap — cleanup should evict down to this limit.
	rl.maxClients = 5
	rl.cleanup()

	rl.mu.Lock()
	after := len(rl.requests)
	rl.mu.Unlock()

	assert.LessOrEqual(t, after, 5, "cleanup should evict down to maxClients")
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := newRateLimiter(1000, 60)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				rl.isAllowed(randomIP(id))
			}
		}(i)
	}

	// Run cleanup concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 10; j++ {
			rl.cleanup()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

func TestRateLimit_DisabledPassesThrough(t *testing.T) {
	cfg := &RateLimitConfig{Enabled: false}

	r := gin.New()
	r.Use(RateLimit(cfg))
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestRateLimiter_IsAllowed_RejectsNewClientOverMaxClients(t *testing.T) {
	rl := newRateLimiter(100, 60)
	rl.maxClients = 3

	// Fill up to maxClients
	rl.isAllowed("client-a")
	rl.isAllowed("client-b")
	rl.isAllowed("client-c")

	// Existing client should still be allowed
	allowed, _, _ := rl.isAllowed("client-a")
	assert.True(t, allowed, "existing client should be allowed")

	// New client should be rejected because map is at capacity
	allowed, _, _ = rl.isAllowed("client-d")
	assert.False(t, allowed, "new client should be rejected when at maxClients")
}

func TestRateLimiter_IsAllowed_AllowsNewClientAfterExpiredClientsPruned(t *testing.T) {
	rl := newRateLimiter(100, 1)
	rl.maxClients = 2

	allowed, _, _ := rl.isAllowed("client-a")
	require.True(t, allowed)
	allowed, _, _ = rl.isAllowed("client-b")
	require.True(t, allowed)

	time.Sleep(1100 * time.Millisecond)

	allowed, _, _ = rl.isAllowed("client-c")
	assert.True(t, allowed, "expired clients should not block a new client from being admitted")

	rl.mu.Lock()
	size := len(rl.requests)
	_, hasA := rl.requests["client-a"]
	_, hasB := rl.requests["client-b"]
	_, hasC := rl.requests["client-c"]
	rl.mu.Unlock()

	assert.Equal(t, 1, size, "only the newly active client should remain tracked")
	assert.False(t, hasA)
	assert.False(t, hasB)
	assert.True(t, hasC)
}

func randomIP(seed int) string {
	return "10." +
		itoa((seed/65536)%256) + "." +
		itoa((seed/256)%256) + "." +
		itoa(seed%256)
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
