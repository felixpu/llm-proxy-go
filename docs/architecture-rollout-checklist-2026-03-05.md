# Architecture Rollout Checklist (2026-03-05)

Scope: architecture review items `#11 #9 #13` (high-risk structural changes).

## 1. Pre-Deploy Gate

- [x] Unit/regression passed locally: `go test ./internal/... ./cmd/...`
- [ ] Build artifact generated in CI
- [ ] Release tag/version confirmed
- [ ] Rollback artifact ready (previous stable build/version)

## 2. Canary Plan

- [ ] Deploy to canary environment / small traffic slice
- [ ] Start time recorded: `____`
- [ ] Planned observation window: `>= 60 minutes`

## 3. Runtime Verification

- [ ] API 5xx ratio does not exceed baseline + threshold
- [ ] p95 latency on proxy routes does not exceed baseline + threshold
- [ ] No abnormal spike in circuit transitions (open/reopen)
- [ ] Log list/stats/aggregation outputs match baseline query set
- [ ] Cache behavior sanity check passes (hit/miss/expiry no obvious drift)

## 4. Hard Rollback Triggers

Rollback immediately if any occurs:

1. 5xx ratio breaches threshold.
2. p95 latency on impacted routes breaches threshold.
3. Circuit breaker abnormal transitions spike.
4. Log statistics/aggregation mismatch against baseline query set.

## 5. Rollback Procedure

1. Switch traffic back to previous stable build (or redeploy previous version).
2. Verify API health and key metrics return to baseline.
3. Record incident notes:
   - trigger time
   - symptom
   - suspected package (`#11/#9/#13`)
4. Freeze further rollout until fix PR is prepared.

## 6. Completion Criteria

- [ ] Canary window passed with no rollback trigger
- [ ] Full rollout completed
- [ ] Post-rollout metrics stable for one full monitoring window
- [ ] Final notes documented in release/PR record
