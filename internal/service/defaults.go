package service

import "time"

const (
	// DefaultRoutingCacheSize is the default capacity for in-memory routing cache.
	DefaultRoutingCacheSize = 10000

	// DefaultAsyncRepoTimeout is the timeout used by async best-effort repository updates.
	DefaultAsyncRepoTimeout = 5 * time.Second

	// DefaultProxyHTTPTimeout is the non-stream HTTP timeout for proxy upstream requests.
	DefaultProxyHTTPTimeout = 120 * time.Second

	// DefaultProxyResponseMaxBytes limits non-stream success response size.
	DefaultProxyResponseMaxBytes = 32 * 1024 * 1024 // 32 MiB

	// DefaultProxyErrorResponseMaxBytes limits captured upstream error body size.
	DefaultProxyErrorResponseMaxBytes = 2 * 1024 * 1024 // 2 MiB

	// DefaultContentPreviewMaxChars is the max length for persisted/logged content preview.
	DefaultContentPreviewMaxChars = 200

	// DefaultCacheCleanupInterval is the interval for cache background cleanup.
	DefaultCacheCleanupInterval = 5 * time.Minute

	// DefaultShadowRoutingTimeout is the max timeout used by async shadow routing calls.
	DefaultShadowRoutingTimeout = 3 * time.Second
)
