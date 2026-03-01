# Next Steps

## Terminology

Keep 5 baseball terms:

| Term | Purpose | Location |
|------|---------|----------|
| **Umpire** | Orchestrates validation | `tools/umpire/umpire.go` |
| **Pitcher** | Fault injection | `tools/umpire/pitcher/` |
| **Scorebook** | Event recording | `tools/umpire/scorebook/` |
| **Lineup** | Entity registry | `tools/bats/lineup/` |
| **Rulebook** | Property models | `tools/umpire/rulebook/` |

Drop: Play (use "Fault"), Game, Catcher (fold into Umpire), Scout, Skipper, Roster.

---

## Priority 1: Make Fault Injection Work

### Step 1: Complete Pitcher Matching

**File**: `tools/umpire/pitcher/pitcher.go:205-214`

The matcher is stubbed - always returns false. Need to:
- Extract WorkflowID, NamespaceID, TaskQueue, RunID from gRPC request
- Compare against MatchCriteria fields
- Return true if all non-empty criteria match

This unblocks targeted fault injection.

### Step 2: Integration Test Proving the Loop

**File**: `tests/umpire/fault_injection_test.go` (new)

Demonstrate:
1. Configure pitcher with MatchCriteria targeting workflow "test-123"
2. Start workflow "test-123"
3. Verify fault injected (check scorebook)
4. Verify workflow recovered
5. Run umpire.Check() - no violations

---

## Priority 2: Implement Workflow Update Rules

**Location**: `tools/umpire/rulebook/rules/workflow_update/`

5 rules to implement (currently placeholders):

| Rule | Detects |
|------|---------|
| UpdateDeduplication | Same update processed twice |
| UpdateCompletion | Update stuck in accepted state |
| SpeculativeTaskRollback | Speculative execution not cleaned up |
| UpdateLossPrevention | Update disappeared without completion |
| UpdateStateConsistency | Invalid state transitions |

Implement in order of bug likelihood:
1. UpdateCompletion - simplest, clearest invariant
2. UpdateLossPrevention - updates shouldn't vanish
3. UpdateDeduplication - duplicates cause corruption
4. UpdateStateConsistency - FSM violations
5. SpeculativeTaskRollback - cleanup edge cases

---

## Priority 3: Demo-Ready Output

### Add Violation Formatter

Human-readable violation output for demos:
- Timeline of events leading to violation
- Clear description of what went wrong
- Entity state at time of violation

Even simple pretty-printed JSON helps for business case.

### Write Bug-Catching Test

Test that **intentionally triggers a violation**:
- Inject a fault that causes incorrect behavior
- Show umpire detecting and reporting it

This is the money shot for the business case.

---

## Architecture Summary

```
Test
  ├── Configure faults (Pitcher)
  ├── Run workflow
  ├── Record events (Scorebook)
  ├── Update entities (Lineup)
  └── Check rules (Umpire + Rulebook)
       └── Report violations
```

Hierarchical identity system (`namespace:ns1/workflow:wf1/execution:run1`) enables:
- Parent-child entity relationships
- Scoped queries (all tasks for a workflow)
- Clear entity addressing

---

## Effort Estimate

| Step | Effort |
|------|--------|
| Pitcher matching | 1-2 days |
| Integration test | 0.5 day |
| 5 update rules | 3-4 days |
| Violation formatter | 0.5 day |
| Bug-catching test | 0.5 day |

**POC complete: ~6-8 days**

---

## Files to Update

### Delete from PLAN_ARCHITECTURE.md
- Scout section
- Skipper section
- References to Game abstraction

### Consolidate Terminology
- Use "Lineup" not "Roster" for entities
- Use "Fault" not "Play" for injection configs
- Fold Catcher functionality into Umpire description
