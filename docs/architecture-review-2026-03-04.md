# LLM Proxy Go Architecture Review (2026-03-04)

## Summary

- Total optimization items identified: `24`
- Priority split:
  - `P0`: 5 (must fix first)
  - `P1`: 9 (high priority)
  - `P2`: 10 (medium priority)
- Current architecture maturity (estimated): `6.5/10`

## Full Checklist

### P0 (Must Fix First)

1. API fallback semantics are broken for unknown `/api/*` routes.
   - Impact: API clients may receive `200 + HTML` instead of JSON error.
   - Location: `internal/api/server.go:299`, `internal/api/server.go:300`

2. Test infra drift: duplicated testutil schema has diverged.
   - Impact: different test suites validate against different schema assumptions.
   - Location: `internal/testutil/db.go:97`, `tests/testutil/db.go:97`, `internal/testutil/db.go:180`, `tests/testutil/db.go:180`

3. HealthChecker lifecycle is not idempotent.
   - Impact: repeated Start/Stop can lead to channel close hazards.
   - Location: `internal/service/health_checker.go:138`, `internal/service/health_checker.go:193`

4. Routing analysis task state has no cleanup strategy.
   - Impact: `tasks` map can grow indefinitely in long-running service.
   - Location: `internal/service/routing_analyzer.go:31`, `internal/service/routing_analyzer.go:52`, `internal/service/routing_analyzer.go:92`

5. DIP violation in core service construction.
   - Impact: LLMRouter hard-couples to concrete repositories, reducing replaceability/testability.
   - Location: `internal/service/llm_router.go:23`, `internal/service/llm_router.go:40`, `internal/service/llm_router.go:45`

### P1 (High Priority)

6. `Server` composition root is over-burdened (SRP issue).
   - Location: `internal/api/server.go:48`

7. `ServerDeps` mixes interfaces and concrete types (unstable boundary).
   - Location: `internal/api/server.go:22`

8. `ProxyHandler` has too many responsibilities.
   - Scope: auth, validation, endpoint selection, streaming orchestration, content logging policy.
   - Location: `internal/api/handler/proxy.go:46`, `internal/api/handler/proxy.go:129`, `internal/api/handler/proxy.go:218`, `internal/api/handler/proxy.go:407`

9. `HealthChecker` mixes probing/metrics/circuit-breaking/error classification.
   - Location: `internal/service/health_checker.go:115`, `internal/service/health_checker.go:386`, `internal/service/health_checker.go:584`

10. `LLMRouter` mixes orchestration with infra concerns.
    - Location: `internal/service/llm_router.go:57`

11. `RequestLogRepositoryImpl` is too broad.
    - Scope: write path + list/query + aggregate statistics + analysis feed.
    - Location: `internal/repository/requestlog_repo_impl.go:37`, `internal/repository/requestlog_repo_impl.go:129`, `internal/repository/requestlog_repo_impl.go:503`, `internal/repository/requestlog_repo_impl.go:622`

12. Cache monitor routes are registered twice.
    - Location: `internal/api/server.go:278`, `internal/api/server.go:291`

13. Redundant cache implementations coexist.
    - Location: `internal/service/routing_cache.go:73`, `internal/service/cache_service.go:74`, `cmd/llm-proxy/main.go:161`

14. Same request path reads routing config multiple times.
    - Location: `internal/service/endpoint_selector.go:66`, `internal/service/llm_router.go:59`, `internal/api/handler/proxy.go:412`

### P2 (Medium Priority)

15. `EndpointStore` exposes internal slice (encapsulation leak).
    - Location: `internal/service/endpoint_store.go:86`

16. `EndpointStore` depends on concrete repositories.
    - Location: `internal/service/endpoint_store.go:18`

17. Context key uses raw string (`"endpoints"`), type-safety risk.
    - Location: `internal/api/server.go:64`, `internal/api/handler/proxy.go:104`

18. Type assertion on context value is not guarded.
    - Location: `internal/api/handler/proxy.go:116`

19. Repository interfaces leak implementation-level DTOs.
    - Location: `internal/repository/interfaces.go:80`, `internal/repository/interfaces.go:85`, `internal/repository/interfaces.go:93`

20. `map[string]any` update contract is weakly typed.
    - Location: `internal/repository/interfaces.go:19`, `internal/repository/interfaces.go:30`, `internal/repository/interfaces.go:67`

21. `main.run` remains a large startup function.
    - Location: `cmd/llm-proxy/main.go:63`

22. Background goroutines lack unified stop lifecycle in some flows.
    - Location: `internal/service/llm_router.go:122`, `internal/service/cache_service.go:95`, `internal/service/cache_service.go:342`

23. Strategy/config constants are scattered across modules.
    - Location: `internal/service/proxy.go:55`, `internal/service/routing_analyzer_v2.go:45`, `internal/service/cache_service.go:16`

24. Routing analyzer has dual implementation tracks (v1/v2) without full convergence.
    - Location: `internal/service/routing_analyzer.go:22`, `internal/service/routing_analyzer_v2.go:13`

## Principle Mapping

- SRP issues: 6
- DIP issues: 4
- DRY issues: 4
- OCP pressure points: 3
- Interface/boundary design issues: 2
- Lifecycle/concurrency issues: 3
- Performance/path efficiency issues: 2

## 3-Phase Refactor Plan

### Phase 1 (1-2 days, risk control)

1. Fix API fallback behavior: `/api/*` unmatched -> JSON 404.
2. Merge and unify testutil schema source.
3. Add TTL/size cleanup for `RoutingAnalyzer.tasks`.
4. Make `HealthChecker` Start/Stop idempotent.

### Phase 2 (3-5 days, reduce coupling)

1. Refactor `LLMRouter` to interface-driven constructor injection.
2. Convert `ServerDeps` to interface-first dependency boundary.
3. Extract proxy content logging policy from `ProxyHandler`.
4. Add request-scope routing config cache (single read per request).

### Phase 3 (5-10 days, architecture cleanup)

1. Split `RequestLogRepositoryImpl` into write/query/analytics repos.
2. Split `HealthChecker` into probe, circuit-breaker, metrics modules.
3. Converge cache architecture (`RoutingCache` vs `CacheService`).
4. Split route registration by domain in `internal/api/server.go`.

## Regression Test Focus

1. `go test ./internal/...` must remain green.
2. Add API contract test for unmatched `/api/*` route.
3. Add idempotent lifecycle tests for HealthChecker Start/Stop.
4. Add task-retention tests for routing analysis cleanup behavior.
