# Workflow Update Rules

This directory contains verification rules for workflow update behavior based on properties derived from `tests/update_workflow_test.go`.

## Rules Overview

### 1. Update Deduplication (`update_deduplication.go`)

**Property**: Updates with the same ID are properly deduplicated.

**From Tests**:
- `TestScheduledSpeculativeWorkflowTask_DeduplicateID`
- `TestStartedSpeculativeWorkflowTask_DeduplicateID`

**Verifies**:
- Multiple requests with the same update ID return the same result
- Only one update acceptance/completion event is written to history
- Second update with same ID is deduplicated, not re-executed

**Status**: Placeholder - requires WorkflowUpdate entity implementation

---

### 2. Update Completion (`update_completion.go`)

**Property**: Accepted updates eventually complete or explicitly fail.

**From Tests**:
- `TestEmptySpeculativeWorkflowTask_AcceptComplete`
- `TestNotEmptySpeculativeWorkflowTask_AcceptComplete`
- `TestFirstNormalScheduledWorkflowTask_AcceptComplete`
- `TestNormalScheduledWorkflowTask_AcceptComplete`

**Verifies**:
- Updates that reach ACCEPTED stage transition to COMPLETED
- Updates don't remain in ACCEPTED state indefinitely
- Poll requests eventually see completion
- History contains both Accepted and Completed events

**Violation Conditions**:
- Update accepted but not completed after 30 seconds (default window)
- Update in limbo state without explicit rejection

**Status**: Placeholder - requires WorkflowUpdate entity with lifecycle tracking

---

### 3. Speculative Task Rollback (`speculative_task_rollback.go`)

**Property**: Failed speculative workflow tasks properly roll back associated updates.

**From Tests**:
- `TestSpeculativeWorkflowTask_Fail`
- `TestRunningWorkflowTask_NewEmptySpeculativeWorkflowTask_Rejected`
- `TestRunningWorkflowTask_NewNotEmptySpeculativeWorkflowTask_Rejected`

**Verifies**:
- Failed speculative tasks don't persist updates to history
- Metrics track commits vs rollbacks correctly
- Rejected speculative tasks have updates retried or failed

**Rollback Triggers**:
- Worker explicitly fails the workflow task
- Workflow task times out (ScheduleToStart or StartToClose)
- Speculative task rejected due to concurrent normal task
- Worker connection lost during processing

**Status**: Placeholder - requires WorkflowTask entity with speculative tracking

---

### 4. Update Loss Prevention (`update_loss_prevention.go`)

**Property**: Updates are preserved through graceful registry operations.

**From Tests**:
- `TestScheduledSpeculativeWorkflowTask_LostUpdate`
- `TestStartedSpeculativeWorkflowTask_LostUpdate`
- `TestFirstNormalWorkflowTask_UpdateResurrectedAfterRegistryCleared`

**Expected Behavior**:

**Graceful Shutdown** (`clearUpdateRegistryAndAbortPendingUpdates`):
- In-flight updates are aborted with errors returned to clients
- Frontend can retry update requests
- Updates eventually delivered after workflow recovers
- ❌ **Violation**: Updates lost instead of aborted

**Crash Scenario** (`loseUpdateRegistryAndAbandonPendingUpdates`):
- Shard finalizer doesn't run (simulated crash)
- Pending updates time out
- ✅ **Acceptable**: Permanent loss in crash scenario

**Status**: Placeholder - requires update registry tracking and graceful shutdown detection

---

### 5. Update State Consistency (`update_state_consistency.go`)

**Property**: Update lifecycle stages follow valid state transitions.

**From Tests**: All update tests verify state consistency via WaitPolicy

**Valid Lifecycle**:
```
UNSPECIFIED -> ADMITTED -> ACCEPTED -> COMPLETED
             ↘         ↘         ↘
                   REJECTED
```

**Valid Transitions**:
- `UNSPECIFIED -> ADMITTED` (received by history)
- `ADMITTED -> ACCEPTED` (worker accepts)
- `ACCEPTED -> COMPLETED` (worker completes with result)
- `Any -> REJECTED` (worker or system rejects)

**Invalid Transitions (Violations)**:
- `ACCEPTED -> ADMITTED` (regression)
- `COMPLETED -> ACCEPTED` (regression)
- `COMPLETED -> REJECTED` (change after completion)
- `REJECTED -> ACCEPTED` (cannot un-reject)

**Status**: Placeholder - requires WorkflowUpdate entity with state transition tracking

---

## Implementation Status

All rules are currently **placeholders** with TODO comments indicating what needs to be implemented:

1. **WorkflowUpdate Entity**: Track update lifecycle, state, acceptance/completion times
2. **Enhanced WorkflowTask Entity**: Add IsSpeculative flag, RolledBackAt timestamp
3. **Update Registry Events**: Detect registry clears (graceful vs crash)
4. **OTEL Span Integration**: Import update-related spans as moves

## Architecture Notes

Following STAMP principles (see `tools/umpire/STAMP.md`):
- ✅ **Properties**: Defined as checkable assertions on entity state
- ✅ **Entities**: Track workflow update lifecycle
- ✅ **Verification**: Rules query entities and return violations
- ❌ **No Action Generation**: CATCH observes real events from OTEL traces

## Next Steps

1. Design WorkflowUpdate entity with lifecycle tracking
2. Create update-related moves from OTEL spans
3. Implement actual violation detection logic in each rule
4. Add unit tests for each rule
5. Register rules in umpire initialization

## References

- Test File: `/tests/update_workflow_test.go`
- STAMP Documentation: `tools/umpire/STAMP.md`
- Existing Rules: `tools/umpire/rulebook/rules/lost_task.go` (reference implementation)
