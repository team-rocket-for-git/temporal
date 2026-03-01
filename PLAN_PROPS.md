Here’s a distilled “property spec” for the *behaviour* in `update_workflow_test.go`, split into safety vs liveness, framed so you can re-express it as property-based tests instead of a giant pile of scenario tests.

I’ll use the Temporal-ish terminology from the file:

* **WT / WFT** = workflow task
* **Speculative WT** = WT created optimistically to deliver updates
* **Update lifecycle**: `ADMITTED → ACCEPTED → COMPLETED` or `REJECTED` or `ABORTED/TIMED_OUT`
* “Registry” = server-side update registry in the shard

---

## 1. Safety properties (nothing bad / inconsistent happens)

### S1. At-most-once application per Update ID

For any `(WorkflowID, RunID, UpdateID)`:

1. **At most one ACCEPTED event and at most one COMPLETED/REJECTED event** exist in history for that UpdateID.
2. If a COMPLETED or REJECTED event exists, it:

   * References **exactly one** Accepted event via `AcceptedEventId` (or equivalent metadata).
   * Has `Meta.UpdateId` equal to the UpdateID of the request.
3. Multiple client calls with the same UpdateID:

   * Either all return the **same outcome** (success / failure / rejection),
   * Or some fail fast with a “duplicate / already finished” style error, but none produce a conflicting outcome. ([GitHub][1])

This is what the various `*_DeduplicateID` tests and the “completed update” cases in `TestCompleteWorkflow_AbortUpdates` enforce.

---

### S2. Speculative WTs obey commit/rollback semantics in history

Think: “speculative WT is a transaction on history.”

1. **Empty speculative WT** (no non-update events):

   * If its update is **accepted and completed**, its WT events **appear** in history at commit time.
   * If its update is **rejected or aborted** and it shipped *no other events*, its WT events **do not appear** in history; history is rolled back to the last committed non-speculative WT. ([GitHub][1])

2. **Non-empty speculative WT** (also schedules activities / signals etc.):

   * Its non-update events (e.g. `ActivityTaskScheduled`, `WorkflowExecutionSignaled`) **must remain** in history even if the update is rejected/aborted.
   * The WT itself is recorded as a normal WT once it is committed or once it is converted to a normal task due to:

     * Buffered signal,
     * Timeout,
     * Heartbeat,
     * Termination,
     * Sticky timeout, etc. ([GitHub][1])

So a property-based test could randomly generate speculative/non-speculative WTs plus “outcomes” (`accept`, `reject`, `timeout`, `convert-to-normal`), then check:

* **Rollback**: empty speculative + rejection ⇒ no speculative WT events in history.
* **Commit**: any speculative that shipped non-update events ⇒ those events stay visible in history, regardless of update outcome.

---

### S3. History ↔ event ID references are consistent

For any update-related history:

1. `WorkflowExecutionUpdateAccepted.AcceptedRequestSequencingEventId` is always an event ID of a `WorkflowTaskScheduled` that carried that update message to the worker.
2. `WorkflowExecutionUpdateCompleted.AcceptedEventId` points to the corresponding Accepted event.
3. When updates complete out of order, each `Completed` still references the correct `Accepted`. ([GitHub][1])

Property: given a randomly generated history (or model trace) that includes update events, check that all these cross-references point to valid event IDs, are non-ambiguous, and never cross-run.

---

### S4. Normal vs speculative WT creation rules

The tests encode a few invariants about *when* speculative WTs are allowed:

1. If there is already a **scheduled/started normal WT** (e.g. due to a signal), sending an update:

   * Delivers it on that normal WT,
   * Does **not** introduce an extra speculative WT. ([GitHub][1])

2. When a signal or activity completion **persists mutable state** while a speculative WT exists, that speculative WT is converted to normal and:

   * Its events are written into history (even if the update is rejected),
   * The update’s rejection/acceptance doesn’t remove those events.

Safety property: in the model, if a non-update event persists the mutable state while a speculative WT is “open”, that WT must transition to “normal”, and later history must show it.

---

### S5. Timeouts and stale workflow task tokens are detected and safe

There are multiple flavours:

1. **Start-to-close or schedule-to-start timeouts**:

   * Speculative WT timeout yields `WorkflowTaskTimedOut` plus a new `WorkflowTaskScheduled` with increased attempt or changed task queue metadata.
   * Update is **not lost**: it remains in the registry and will be re-delivered on the new WT (or will time out at the client). ([GitHub][1])

2. **Stale task tokens** (different StartedEventId or different startTime after context reload):

   * Completing an old WT with mismatched `StartedEventId` or `startTime` must fail with `NotFound("Workflow task not found")`.
   * Only the “current” WT can successfully apply update commands. ([GitHub][1])

Safety property: in the model, if you:

* Clear context / update registry and create a new WT,
* Then attempt to respond using *old* WT token,

…server must reject that response and never apply its commands.

---

### S6. Workflow closure & updates: no cross-run or post-close updates

Across the matrix in `TestCompleteWorkflow_AbortUpdates` and the `Terminate` / `ContinueAsNew` tests:

1. Once a run is **closing** (terminate / fail / complete / continue-as-new):

   * **Newly admitted** updates for that run must be aborted with a retriable “workflow is closing” style error.
   * **Already accepted** updates for that run must fail with a *non-retriable* update failure (“workflow completed before the update completed”), but history remains consistent.
   * **Completed** updates are unaffected. ([GitHub][1])

2. Updates are **not silently carried over** to a new run after continue-as-new; only retried API calls re-target the new run.

Safety property: for each run:

* No Accepted/Completed events for an update appear **after** the terminal event of that run.
* For an update originally targeting run R1, any post-close attempts to complete it must error; if it’s retried, the new attempt must target a fresh update entry on the new run (different UpdateID or explicitly re-admitted).

---

### S7. Update registry semantics: graceful vs crashy loss

Two helper behaviours:

* `clearUpdateRegistryAndAbortPendingUpdates` – “graceful” shard close: context cleared, pending updates aborted and retried.
* `loseUpdateRegistryAndAbandonPendingUpdates` – registry lost but context not cleared; pending updates simply time out. ([GitHub][1])

Safety properties:

1. After **graceful clear**, no “ghost” updates exist:

   * Either they are resurrected from acceptance messages when the worker replies,
   * Or new attempts are admitted and delivered on new WTs.
   * No update is applied without a corresponding acceptance in the rebuilt registry.

2. After **crashy loss**, pending updates:

   * Eventually time out at the client,
   * Are never applied,
   * Do not appear as messages on future WTs.

This is what the `LostUpdate` and “resurrected after registry cleared” tests assert.

---

### S8. Worker that ignores updates must cause server-driven rejection

`TestSpeculativeWorkflowTask_WorkerSkippedProcessing_RejectByServer` gives:

* If a worker repeatedly completes WTs **without** consuming update messages for a given UpdateID, the server must:

  * Reject the update with a well-defined failure (“worker did not process update / update not supported”),
  * Reset history appropriately (using `ResetHistoryEventId`),
  * Allow *other* updates (e.g., from a newer worker) to succeed afterwards. ([GitHub][1])

Safety property: the system does not leave updates “half applied” if the worker doesn’t look at them; we either commit or explicit-reject.

---

### S9. Query buffer overflow & WF context clear do not strand updates

`TestSpeculativeWorkflowTask_QueryFailureClearsWFContext`:

* When queries overflow (“query buffer is full”), the WF context is cleared, *and* the update registry must be cleared with it, so that:

  * Pending updates are retried by the frontend,
  * New speculative WTs are created,
  * Update can still be applied and completed successfully. ([GitHub][1])

Safety property: clearing context cannot strand an admitted/accepted update in an unreachable state.

---

### S10. History shape around final WTs with updates

* If the workflow completes on the same WT that carries an update, but the update’s lifecycle is still incomplete (client only waited for ACCEPTED), the update may still end in stage COMPLETED with a *failure* describing that the workflow completed before the update. History must reflect:

  * `WorkflowExecutionUpdateAccepted`
  * `WorkflowExecutionUpdateCompleted` (with failure)
  * `WorkflowExecutionCompleted`
    in that order. ([GitHub][1])

Safety property: update outcomes and workflow closure events are **consistent and ordered**; the client never gets a success for an update that was cut off by workflow completion.

---

## 2. Liveness properties (something eventually happens)

Now the “eventually” style properties that the tests implicitly encode.

### L1. Every admitted update eventually either completes, rejects, times out, or is aborted with a clear reason

For any admitted update under a “reasonable” environment (no infinite crashes, shards eventually alive, worker eventually responds):

* The client eventually observes one of:

  * **Success outcome** (COMPLETED with `Success`),
  * **Rejected** (COMPLETED with `Failure` that is a rejection),
  * **Aborted** with an explicit reason (`workflow closing`, `worker did not process update`, etc.),
  * **Timeout** (context deadline exceeded on client). ([GitHub][1])

There is no state where an update is forever ADMITTED/ACCEPTED but never makes progress: all the tests around timeouts, registry clearing, query failures, etc., push towards “some terminal outcome or explicit timeout”.

In a property-based model you’d say: *if* you keep driving the system (pumping WTs, simulating retries) and **don’t** permanently kill the worker / shard, every update reaches a terminal state.

---

### L2. Updates are delivered to workers in admission order

`TestUpdatesAreSentToWorkerInOrderOfAdmission` encodes:

* If the server admits updates `u0, u1, …, u(n-1)` in that order, then the first WT that delivers these updates will carry the messages in that **same order**, and history will show Accepted/Completed in an order consistent with admission. ([GitHub][1])

Liveness property: the scheduler doesn’t starve older updates – they’re not allowed to be delayed indefinitely behind newer ones.

---

### L3. Speculative → normal WT conversions still let updates finish

Across the tests that:

* Convert speculative WTs because of signals,
* Because of activity completions,
* Because of heartbeats,
* Because of sticky or schedule-to-start timeouts,

…if the worker eventually returns a valid completion, the update **still completes successfully** (unless the workflow closes first). ([GitHub][1])

Liveness: speculation + conversion doesn’t “lose” an update; it may change which WT commits it, but the update will eventually reach completed/rejected as long as the worker cooperates.

---

### L4. Deduplication does not block progress

For all the `*_DeduplicateID` tests:

* Re-sending the same UpdateID:

  * Must **not** create new WTs forever, and
  * Must **not** change the result,
  * Must either:

    * Return **instantly** with the already-known result (if the first one completed),
    * Or share the “in-flight” lifecycle of the first attempt.

Liveness property: deduplication prevents *extra* work but never causes an update to stall; a second caller always gets either an in-flight future that will resolve, or an already computed outcome.

---

### L5. Continue-as-new + retries allow an update to reach new run

In the continue-as-new tests (at the end of the file):

* An update that was only **admitted** on the old run and aborted with a “workflow closing” error is **retriable**, and when retried by SDK it eventually lands on the new run and progresses there.

Liveness property: continue-as-new does not “kill” updates that the SDK is still retrying; they eventually land somewhere they can finish.

---

### L6. Polling for ACCEPTED may observe COMPLETED (monotone stage)

`TestWaitAccepted_GotCompleted`:

* If the client’s wait policy is “wait until ACCEPTED”, but by the time the result returns the update is already COMPLETED, the poll returns **stage COMPLETED with full outcome**.

Liveness/invariant hybrid: the stage is monotone (never goes backwards), and the client is never forced to re-poll to discover completion – it’s pushed with the best-known stage.

---

## 3. How to turn this into property-based tests

A practical way to wrap this suite into properties:

1. **Model-level generator**:

   * Generate a sequence of operations:

     * `StartWorkflow`,
     * `SendUpdate(updateId, waitPolicy)`,
     * `SendSignal`,
     * `ScheduleActivity/CompleteActivity`,
     * `Terminate`, `ContinueAsNew`,
     * `LoseRegistry`, `ClearRegistry`,
     * Random **worker behaviours** (proper processing, ignore updates, malformed response, slow, etc.),
     * Random **timeouts** and **query load**.
   * Run against a **small in-memory model** of the Temporal history & registry, or against a real test cluster if you can.

2. **For each step, assert safety properties**:

   * No duplicate Accepted/Completed per UpdateID.
   * Event ID references are valid.
   * Speculative WT rules (commit/rollback).
   * Stale token detection (when you simulate context reload).
   * Behaviour at workflow close.

3. **After a bounded “drive” of the system, assert liveness properties**:

   * All updates that are not actively blocked by you closing the workflow or killing the worker either:

     * Have a terminal outcome,
     * Or the client context has timed out.

4. **Shrinkable counterexamples**:

   * When a property fails, you get a short sequence (like one of the existing tests) instead of hand-written scenario.

If you want, I can next:

* Pick **one** of these properties (e.g. “speculative commit/rollback on reject vs non-empty”)
* And sketch an actual QuickCheck/GoCheck-like property with a tiny state machine model to drive Temporal.

[1]: https://raw.githubusercontent.com/temporalio/temporal/refs/heads/main/tests/update_workflow_test.go "raw.githubusercontent.com"
