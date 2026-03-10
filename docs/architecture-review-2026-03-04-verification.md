# Architecture Review Verification (2026-03-04)

Based on `docs/architecture-review-2026-03-04.md`, all 24 items were cross-checked against current code.

## Result Summary

- Total reviewed: `24/24`
- `Confirmed`: 19
- `Partially Confirmed`: 5
- `Rejected`: 0

## Item-by-Item Verification

### P0

1. API fallback semantics issue for `/api/*` unmatched routes  
   - Status: `Confirmed`  
   - Evidence: `internal/api/server.go:299-300` uses global `NoRoute(handler.ServeFrontend())`.

2. Test infra drift between `internal/testutil` and `tests/testutil`  
   - Status: `Confirmed`  
   - Evidence: `custom_headers` differs (`internal/testutil/db.go:97` vs `tests/testutil/db.go:97`), and `cross_role_fallback_enabled` exists only in `tests/testutil/db.go:180`.

3. HealthChecker Start/Stop idempotency risk  
   - Status: `Confirmed`  
   - Evidence: `done` initialized once (`internal/service/health_checker.go:138`) and closed in loop (`internal/service/health_checker.go:193`), but not recreated in `Start`.

4. RoutingAnalyzer task map lacks retention/cleanup  
   - Status: `Confirmed`  
   - Evidence: `tasks` map initialized (`internal/service/routing_analyzer.go:52`) and inserted (`internal/service/routing_analyzer.go:92`) without deletion path.

5. DIP violation in LLMRouter construction  
   - Status: `Confirmed`  
   - Evidence: constructor directly creates repos (`internal/service/llm_router.go:40-45`).

### P1

6. `Server` composition root is over-burdened  
   - Status: `Confirmed`  
   - Evidence: `internal/api/server.go:48-297` contains middleware setup, dependency wiring, and all route registration.

7. `ServerDeps` mixes abstractions and concrete types  
   - Status: `Confirmed`  
   - Evidence: same struct contains interfaces (`UserRepo`) and concrete implementations (`*repository.SQLModelRepository`) at `internal/api/server.go:22-45`.

8. `ProxyHandler` has too many responsibilities  
   - Status: `Confirmed`  
   - Evidence: auth + validation + selection + proxy + error mapping + logging policy in `internal/api/handler/proxy.go:46-455`.

9. `HealthChecker` responsibilities are too broad  
   - Status: `Confirmed`  
   - Evidence: probing (`checkEndpoint`), stats/circuit logic (`UpdateRequestStatsV2`), classification (`classifyHTTPError`) in one type (`internal/service/health_checker.go`).

10. `LLMRouter` responsibilities are too broad  
   - Status: `Confirmed`  
   - Evidence: config load, rule routing, cache orchestration, LLM call orchestration in `internal/service/llm_router.go:57+`.

11. `RequestLogRepositoryImpl` breadth is too large  
   - Status: `Confirmed`  
   - Evidence: write path (`Insert`), list/stats (`List`, `GetStatistics`), analytics (`GetRoutingAggregation`, `ListForAnalysis`, `GetEndpointModelStats`) in one file.

12. Cache monitor route duplication  
   - Status: `Confirmed`  
   - Evidence: equivalent handlers under both `/api/config/cache/*` and `/api/cache/*` in `internal/api/server.go:278-283` and `291-296`.

13. Redundant cache implementations coexist  
   - Status: `Partially Confirmed`  
   - Evidence: both `RoutingCache` and `CacheService` exist; however, `NewCacheService` is only used in tests (not runtime).  
   - Adjustment: this is currently more of `dead/parallel design debt` than active runtime duplication.

14. Multiple routing config reads on hot request path  
   - Status: `Confirmed`  
   - Evidence: `GetConfig` in endpoint selection (`internal/service/endpoint_selector.go:66`), router (`internal/service/llm_router.go:59`), and proxy content logging (`internal/api/handler/proxy.go:412,442`).

### P2

15. EndpointStore exposes internal slice  
   - Status: `Confirmed`  
   - Evidence: `GetEndpoints` returns `s.endpoints` directly (`internal/service/endpoint_store.go:86-90`).

16. EndpointStore depends on concrete repo types  
   - Status: `Confirmed`  
   - Evidence: fields/ctor use `*repository.SQLModelRepository` and `*repository.SQLProviderRepository` (`internal/service/endpoint_store.go:18-28`).

17. Raw string context key usage  
   - Status: `Confirmed`  
   - Evidence: set/get `"endpoints"` via string key (`internal/api/server.go:64`, `internal/api/handler/proxy.go:104`).

18. Unguarded type assertion from context  
   - Status: `Confirmed`  
   - Evidence: direct cast `endpoints.([]*models.Endpoint)` (`internal/api/handler/proxy.go:116`).

19. Repository interface leaks repository-defined DTOs  
   - Status: `Partially Confirmed`  
   - Evidence: interface returns `*LogStatistics`, `*RoutingAggregation`, `*EndpointModelStats` (`internal/repository/interfaces.go:80,85,93`), and those types are declared in impl file.  
   - Adjustment: this is not an implementation leak in Go package terms, but it does couple service boundary to repository analytics DTOs.

20. `map[string]any` update contracts are weakly typed  
   - Status: `Confirmed`  
   - Evidence: update signatures in `internal/repository/interfaces.go:19,30,67`.

21. `main.run` is a large startup method  
   - Status: `Confirmed`  
   - Evidence: long orchestration block in `cmd/llm-proxy/main.go:63-235`.

22. Background goroutines lack unified shutdown model  
   - Status: `Partially Confirmed`  
   - Evidence: `CacheService` starts perpetual cleanup goroutine (`internal/service/cache_service.go:95`, `342`) with no stop hook; LLMRouter async hit-count goroutine is short-lived and timeout-bound (`internal/service/llm_router.go:122-128`).  
   - Adjustment: true for long-lived background loops, weaker for short-lived fire-and-forget updates.

23. Constants/strategy knobs are scattered  
   - Status: `Partially Confirmed`  
   - Evidence: knobs appear in multiple services (`internal/service/proxy.go`, `internal/service/cache_service.go`, `internal/service/routing_analyzer_v2.go`).  
   - Adjustment: this is maintainability debt, not necessarily design bug.

24. V1/V2 analysis implementation divergence  
   - Status: `Partially Confirmed`  
   - Evidence: `routing_analyzer.go` delegates to methods in `routing_analyzer_v2.go` (`runSinglePassAnalysis`, `runBatchedAnalysis`).  
   - Adjustment: current state is split across files but same receiver (`RoutingAnalyzer`), so it is not two independently active engines.

## Final Conclusion

The original review is directionally accurate.  
Five items should be treated as **scope/priority refinements** rather than hard defects:

- #13 cache duplication
- #19 interface boundary leakage
- #22 goroutine shutdown model
- #23 scattered constants
- #24 v1/v2 divergence

All other items are directly supported by current code and should remain in the remediation backlog.

## Change Quantification Baseline

### Measurement Dimensions

- Changed files
- Estimated LoC (additions + deletions)
- Test impact (new/updated tests)
- Estimated effort (person-days)
- Risk level (Low/Medium/High)

### Phase 1 (P0 stabilization)

1. API NoRoute split for API/SPA
   - Changed files: `2-3`
   - Estimated LoC: `40-80`
   - Test impact: `2-3`
   - Effort: `0.5-1.0 pd`
   - Risk: `Low`
2. Unify duplicated testutil schema source
   - Changed files: `4-6`
   - Estimated LoC: `120-220`
   - Test impact: `3-5`
   - Effort: `1.0-1.5 pd`
   - Risk: `Medium`
3. Add task retention/cleanup in RoutingAnalyzer
   - Changed files: `1-2`
   - Estimated LoC: `80-160`
   - Test impact: `2-4`
   - Effort: `0.5-1.0 pd`
   - Risk: `Medium`
4. HealthChecker Start/Stop idempotency hardening
   - Changed files: `1-2`
   - Estimated LoC: `60-120`
   - Test impact: `2-3`
   - Effort: `0.5-1.0 pd`
   - Risk: `Medium`

Phase 1 total:

- Changed files: `8-13`
- Estimated LoC: `300-580`
- Test impact: `9-15`
- Effort: `2.5-4.5 pd`

### Phase 2 (coupling reduction)

1. Interface-driven LLMRouter construction
   - Changed files: `4-7`
   - Estimated LoC: `180-320`
   - Test impact: `4-6`
   - Effort: `1.5-2.5 pd`
   - Risk: `Medium`
2. Interface-first `ServerDeps` cleanup
   - Changed files: `3-6`
   - Estimated LoC: `120-260`
   - Test impact: `2-4`
   - Effort: `1.0-2.0 pd`
   - Risk: `Medium`
3. Extract proxy content logging policy component
   - Changed files: `3-5`
   - Estimated LoC: `120-220`
   - Test impact: `3-5`
   - Effort: `1.0-1.5 pd`
   - Risk: `Medium`
4. Request-scope routing config cache
   - Changed files: `2-4`
   - Estimated LoC: `80-160`
   - Test impact: `2-3`
   - Effort: `0.5-1.0 pd`
   - Risk: `Low`

Phase 2 total:

- Changed files: `12-22`
- Estimated LoC: `500-960`
- Test impact: `11-18`
- Effort: `4.0-7.0 pd`

### Phase 3 (structural refactor)

1. Split RequestLog repository into write/query/analytics repos
   - Changed files: `6-10`
   - Estimated LoC: `350-700`
   - Test impact: `6-10`
   - Effort: `3.0-5.0 pd`
   - Risk: `High`
2. Split HealthChecker into probe/circuit/metrics modules
   - Changed files: `5-9`
   - Estimated LoC: `300-650`
   - Test impact: `6-10`
   - Effort: `3.0-5.0 pd`
   - Risk: `High`
3. Converge cache architecture (`RoutingCache` vs `CacheService`)
   - Changed files: `4-8`
   - Estimated LoC: `220-500`
   - Test impact: `4-8`
   - Effort: `2.0-4.0 pd`
   - Risk: `High`
4. Route registration modularization in API server
   - Changed files: `4-7`
   - Estimated LoC: `180-360`
   - Test impact: `2-4`
   - Effort: `1.5-3.0 pd`
   - Risk: `Medium`

Phase 3 total:

- Changed files: `19-34`
- Estimated LoC: `1050-2210`
- Test impact: `18-32`
- Effort: `9.5-17.0 pd`

### Whole-Plan Aggregate

- Changed files: `39-69`
- Estimated LoC: `1850-3750`
- Test impact: `38-65`
- Effort: `16.0-28.5 pd`

## Risk Assessment Baseline

### Scoring Model

- Probability (P): `1-5`
- Impact (I): `1-5`
- Risk score = `P x I`
- Bands:
  - `1-6`: Low
  - `8-12`: Medium
  - `15-25`: High

### Highest-Risk Work Packages

1. RequestLog repository split
   - P: `4`, I: `5`, Score: `20` (`High`)
   - Main risk: query behavior drift in list/stats/analytics.
2. HealthChecker modular split
   - P: `4`, I: `5`, Score: `20` (`High`)
   - Main risk: circuit breaker state regression.
3. Cache architecture convergence
   - P: `4`, I: `4`, Score: `16` (`High`)
   - Main risk: cache hit-rate and consistency regression.

### Medium-Risk Work Packages

1. Testutil unification
   - P: `3`, I: `4`, Score: `12` (`Medium`)
2. HealthChecker idempotency hardening
   - P: `3`, I: `4`, Score: `12` (`Medium`)
3. RoutingAnalyzer retention cleanup
   - P: `3`, I: `3`, Score: `9` (`Medium`)
4. LLMRouter interface injection refactor
   - P: `3`, I: `4`, Score: `12` (`Medium`)
5. `ServerDeps` boundary cleanup
   - P: `3`, I: `3`, Score: `9` (`Medium`)
6. Proxy logging policy extraction
   - P: `3`, I: `3`, Score: `9` (`Medium`)
7. Route registration modularization
   - P: `2`, I: `4`, Score: `8` (`Medium`)

### Low-Risk Work Packages

1. API/SPA NoRoute split
   - P: `2`, I: `3`, Score: `6` (`Low`)
2. Request-scope config cache
   - P: `2`, I: `3`, Score: `6` (`Low`)

### Risk Controls (Execution Guardrails)

1. Add contract tests before each change package (baseline first, then refactor).
2. Gate high-risk refactors behind feature flags where possible.
3. Enforce behavior equivalence checks for:
   - endpoint selection output
   - circuit breaker transitions
   - log query and aggregation outputs
4. Require `go test ./internal/...` and focused regression suites per package.

## Mandatory Risk-Control Execution SOP

This SOP is mandatory for **any** architecture-related change in this plan.
No change should be merged if any required step is skipped.

### A. Global Rules (Apply to Every Change)

1. One change package per PR.
2. Do not mix refactor and behavior change in the same commit unless explicitly approved.
3. Every PR must include:
   - target item id from this review (e.g., `#3`, `#11`)
   - risk score (`P x I`)
   - rollback method
   - before/after validation evidence
4. High-risk work (`score >= 15`) requires feature flag or dual-path validation.
5. Any observed unexpected behavior drift must stop the rollout immediately.

### B. Pre-Change Steps (Before Editing Code)

1. Define the change scope:
   - in-scope files
   - out-of-scope files
   - expected behavior unchanged/changed points
2. Build a baseline behavior snapshot:
   - API contract outputs for impacted endpoints
   - key logs/metrics format
   - key DB query outputs (if repo changes)
3. Add or update baseline tests first:
   - contract tests
   - regression tests for bug-prone paths
4. Record baseline artifacts in PR notes:
   - command list
   - key output summaries
   - known assumptions

### C. Implementation Steps (During Code Changes)

1. Use incremental commits:
   - Commit 1: scaffolding/interfaces/tests
   - Commit 2: new implementation (not active by default)
   - Commit 3: switch path (or keep behind flag)
2. Keep old path intact until validation passes.
3. For high-risk modules, use one of:
   - feature flag toggle
   - dual-run + compare (shadow mode)
4. Ensure observability at switch points:
   - add structured logs for path selection
   - include request/task identifiers
5. Do not remove fallback path in the same PR that introduces the new path.

### D. Validation Gates (Must Pass Before Merge)

1. Unit/integration gate:
   - `go test ./internal/...` must pass
   - impacted package tests must pass
2. Contract gate:
   - affected API contract tests must pass
   - no schema response drift unless intentional and documented
3. Behavior-equivalence gate (when refactor intended to be behavior-preserving):
   - endpoint selection output matches baseline
   - circuit breaker state transitions match baseline
   - repository stats/aggregation output matches baseline
4. Performance sanity gate:
   - no major regression in critical request path latency
   - no obvious increase in DB calls for same request path
5. Concurrency/lifecycle gate (if applicable):
   - no goroutine leak symptoms
   - repeated Start/Stop or task lifecycle tests pass

### E. Rollout Steps (After Merge / Runtime Rollout)

1. Low-risk change:
   - direct rollout allowed after gates pass
2. Medium-risk change:
   - canary rollout (small traffic slice / limited environment first)
3. High-risk change:
   - canary + feature flag mandatory
   - keep old path hot-switchable
4. Observe for at least one monitoring window before wider rollout.

### F. Hard Rollback Triggers

Rollback immediately if any condition occurs after switch:

1. 5xx ratio increases beyond agreed threshold.
2. Endpoint selection mismatch rate exceeds threshold.
3. Circuit breaker abnormal transitions spike.
4. Request log statistics mismatch against baseline query set.
5. Significant increase in timeout or p95 latency in impacted routes.

### G. Rollback Procedure (Standard)

1. Switch feature flag to old path (or redeploy previous stable build).
2. Confirm traffic returns to baseline behavior.
3. Capture incident notes:
   - trigger time
   - symptom
   - suspected commit/change package
4. Freeze further rollout for this package.
5. Open a follow-up fix PR with isolated scope.

### H. Package-Level Required Checks

For each package below, these checks are mandatory:

1. `#1 API fallback split`:
   - `/api/*` unmatched returns JSON 404
   - frontend routes still resolve index page
2. `#3 HealthChecker idempotency`:
   - repeated `Start/Stop` does not panic
   - no blocked goroutines after repeated cycles
3. `#4 RoutingAnalyzer task cleanup`:
   - completed/failed tasks expire by TTL
   - running task never deleted during execution
4. `#5/#10 LLMRouter refactor`:
   - inference outputs unchanged on golden test cases
   - fallback behavior unchanged
5. `#11 RequestLog repo split`:
   - list/stat/aggregation baseline query set is bitwise-equivalent or intentionally diff-documented
6. `#9 HealthChecker split`:
   - circuit transition truth table regression tests pass
7. `#13 cache convergence`:
   - cache hit behavior validated on deterministic fixtures
   - stale/expiry behavior validated

### I. Mandatory PR Template Fields

Every PR in this plan must include:

1. Review item ids covered (e.g., `#3`, `#14`)
2. Risk score and risk level
3. Feature flag name (if any)
4. Validation commands executed
5. Baseline vs after comparison summary
6. Rollback command/procedure
7. Deferred risks (if any)

### J. Definition of Done (Risk-Control Complete)

A change package is considered done only when all are true:

1. All required gates passed.
2. Rollback path is verified workable.
3. Monitoring shows no trigger breach in observation window.
4. Documentation updated with:
   - final behavior notes
   - known tradeoffs
   - follow-up debt items (if any).

## Remaining TODO Plan (2026-03-04 Snapshot)

All review items are now implemented in this branch (`#1`-`#24`).
No code-level TODO remains in the architecture remediation backlog.

### Post-Implementation Rollout Checklist

1. Canary deploy changes touching `#11/#9/#13`.
2. Monitor one full observation window for:
   - API error ratio
   - p95 latency on proxy routes
   - endpoint circuit transition anomalies
   - log/statistics query consistency
3. Keep rollback path ready (previous stable build).
4. Execute checklist document:
   - `docs/architecture-rollout-checklist-2026-03-05.md`

### Mandatory Per-PR Checklist (Execution Standard)

1. Include review item ids and risk score.
2. Include rollback method and switch point.
3. Include baseline vs after evidence.
4. Run and report:
   - `go test ./internal/...`
   - impacted package tests
   - required contract/equivalence tests for that package
