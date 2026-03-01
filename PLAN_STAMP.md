# Learning from STAMP

This document analyzes how CATCH can learn from STAMP while keeping things simple.

## What is STAMP?

**STAMP** = write **(S)cenario (T)ests** by sending **(A)ctions** to **(M)odels** and checking their **(P)roperties**.

STAMP is a model-based testing framework where:
- **Models** represent entities (Workflow, TaskQueue, etc.) with state
- **Actions** are sent from actor models to target models via a router
- **Properties** are checkable assertions on model state
- **Scenarios** explore different test paths using generators
- **Router** matches actions to models using reflection-based type routing
- **Verification** happens after every action to check model invariants

### STAMP Architecture

```
┌─────────────────────────────────────────────────┐
│                 Scenario Test                    │
│                                                   │
│  Actor Model ─[Action]→ Router ─→ Target Model  │
│       │                              │           │
│       │                              │           │
│       └──── Verify() ────────────────┘           │
│                                                   │
│  Properties checked via model.Prop[T]            │
│  Scenario.Maybe()/Switch() for path exploration │
└─────────────────────────────────────────────────┘
```

## What is CATCH?

**CATCH** = observability-based property testing framework.

CATCH validates distributed system behavior via:
- **Scout** instruments production code with OTEL spans
- **Catcher** receives OTLP traces
- **Scorebook** converts spans to moves (domain events)
- **Roster** maintains entity state from moves
- **Rulebook** defines property models that query entities
- **Umpire** orchestrates validation
- **Pitcher** injects faults

### CATCH Architecture

```
┌─────────────────────────────────────────────────┐
│              CATCH System                        │
│                                                   │
│  OTEL Spans → Scorebook → Roster (Entities)     │
│                              │                   │
│                              ↓                   │
│                          Rulebook (Models)       │
│                              │                   │
│                              ↓                   │
│                           Umpire                 │
│                                                   │
│  Pitcher injects faults at gRPC intercept level │
└─────────────────────────────────────────────────┘
```

## Problems Both Systems Solve

| Problem | STAMP Approach | CATCH Approach |
|---------|---------------|----------------|
| **Entity Modeling** | Model structs with FSM | Entity structs with FSM |
| **State Tracking** | Updated via OnAction() | Updated via OnEvent() |
| **Property Validation** | Verify() on models | Rulebook models query roster |
| **Action/Event Flow** | Actions sent via Act() | Moves imported from OTLP |
| **Routing** | Reflection-based router | Move type → entity subscriptions |
| **Test Orchestration** | Scenario.Run() with generators | Game → Pitcher → Umpire |
| **Entity Hierarchy** | Model scoping with parent/child | Identity with ParentID |
| **Concurrency** | Lock-free models, env locks | Registry lock, lock-free entities |

## What CATCH Should Learn from STAMP

### 1. ✅ Property System (Already Similar)

STAMP has `Prop[T]` for trackable properties:

```go
type TaskQueue struct {
    Model[*TaskQueue]
    HasTasks Marker  // Prop[bool]
}

func (tq *TaskQueue) Verify() {
    tq.Require.True(tq.HasTasks.Get())
}
```

**CATCH equivalent**: Entities have fields, rulebook models query them:

```go
type TaskQueue struct {
    LastEmptyPollTime time.Time
}

func (m *LostTaskModel) Check(registry EntityRegistry) {
    for _, tq := range registry.QueryEntities(&entities.TaskQueue{}) {
        // Check tq.LastEmptyPollTime
    }
}
```

**Learning**: CATCH's approach is simpler and sufficient. Keep it.

### 2. ❌ Action Generator Pattern (Too Complex for CATCH)

STAMP generates random actions with constraints:

```go
type AddTaskAction struct {
    ActionActor[*WorkerModel]
    ActionTarget[*TaskQueueModel]
    TaskID Gen[string]  // Generated
}

Act(worker, AddTaskAction{
    TaskID: GenName[string](),
})
```

**Why CATCH doesn't need this**:
- CATCH observes *real* events from OTEL traces
- We don't generate synthetic actions
- System-under-test generates its own events

**Don't adopt this**.

### 3. ✅ Router Pattern (But Simplified)

STAMP routes actions to models using reflection:

```go
type MyRouter struct {
    Router
}

// Method names starting with "On" become routes
func (r *MyRouter) OnAddTask(act *AddTaskAction) func(*TaskAddedEvent) {
    tq := Consume[*TaskQueueModel](r, act.TaskQueue)
    return func(evt *TaskAddedEvent) {
        // Handle response
    }
}
```

**CATCH equivalent**: Registry subscribes entities to move types:

```go
registry.RegisterEntity(
    &entities.TaskQueue{},
    entities.NewTaskQueue,
    &moves.AddWorkflowTask{},    // Subscribe to these moves
    &moves.PollWorkflowTask{},
)
```

**Learning**: CATCH's subscription model is simpler than reflection-based routing. Keep it, but consider:
- Move subscriptions could be defined in entity types (currently in registration)
- Could use interfaces like `type SubscribesToPoll interface { OnPoll(...) }`

**Recommendation**: Keep current approach, maybe add interface-based subscriptions later.

### 4. ❌ Model Verification Pattern (Just Removed!)

STAMP calls `Verify()` after every action:

```go
func (env *ModelEnv) Route(act routableAction) {
    // ... route action ...
    env.verify()  // Verify all models
}
```

CATCH just removed `Verify()` from entities!

**Why we removed it**:
- Roster shouldn't know how to verify (separation of concerns)
- Rulebook models define verification logic
- Verification happens at umpire level, not entity level

**Don't revert this**. Keep verification in rulebook.

### 5. ✅ Scenario Testing with Path Exploration

STAMP explores test paths with `Maybe()` and `Switch()`:

```go
func (s *Scenario) Run(testFn func(*Scenario)) {
    s.t.Run("scenario-0", func(t *testing.T) {
        if s.Maybe("fail-matching") {
            // Path A: inject failure
        } else {
            // Path B: no failure
        }
    })
}
```

This automatically generates multiple test cases exploring different paths.

**CATCH equivalent**: Games define scenarios, Skipper selects them:

```go
game := &Game{
    Name: "MatchingFailureRetry",
    PlayConfigs: []PlayConfig{
        {
            Play: play.FailPlay(pitcher.ErrorResourceExhausted),
            Match: MatchCriteria{Target: "matchingservice.AddWorkflowTask"},
        },
    },
}
```

**Learning**: CATCH could benefit from automatic path exploration:

```go
// Future: Scenario API for CATCH tests
func TestWorkflowCompletion(t *testing.T) {
    catch.Scenario(t, func(s *catch.Scenario) {
        if s.Maybe("inject-matching-failure") {
            pitcher.Configure(...)
        }

        if s.Maybe("inject-history-timeout") {
            pitcher.Configure(...)
        }

        // Test runs multiple times exploring all paths
        runWorkflow(...)
        s.Await(workflowCompleted)
    })
}
```

**Recommendation**: Consider adding `catch.Scenario` wrapper with `Maybe()`/`Switch()` for path exploration. Low priority.

### 6. ✅ Future/Async Pattern

STAMP has async actions with futures:

```go
future := ActAsync(actor, action)
result := future.Await()  // Block until complete
```

**CATCH doesn't need this** because:
- Events arrive asynchronously via OTEL
- Rulebook models query final state
- No need to block on individual events

**Don't adopt this**.

### 7. ⚠️ Model Environment Separation

STAMP cleanly separates models from test environment:

```go
type Model[T] struct {
    *internalModel
    Require *require.Assertions
}

type ModelEnv struct {
    logger   log.Logger
    router   routerWrapper
    modelIdx map[modelKey]modelWrapper
}
```

**CATCH equivalent**: Registry holds entities and environment:

```go
type Registry struct {
    logger   log.Logger
    db       *buntdb.DB
    entities map[string]Entity
    children map[string][]string
}
```

**Learning**: CATCH could separate entity registry from persistence:

```go
// Future refactor:
type EntityRegistry struct {
    entities map[string]Entity
    children map[string][]string
}

type EntityPersistence struct {
    db *buntdb.DB
}

type Roster struct {
    registry    *EntityRegistry
    persistence *EntityPersistence
    logger      log.Logger
}
```

**Recommendation**: Consider refactoring for cleaner separation. Medium priority.

### 8. ✅ Entity Identity System

STAMP has scoped model identity:

```go
type Model[T] struct {
    key      modelKey  // "parent/TaskQueue[id-123]"
    domainID ID        // "id-123"
}
```

**CATCH equivalent**: Identity with parent:

```go
type Identity struct {
    EntityID EntityID
    ParentID *EntityID  // Optional parent
}
```

**Learning**: Both systems are similar. CATCH's approach is sufficient.

### 9. ❌ Generator Context with Deterministic Randomness

STAMP uses generators with seeds for reproducible tests:

```go
type genContext struct {
    baseSeed     int
    seedLookup   map[string]int
    pickChoiceFn func(string, int) int
}

Gen[string]{}.Next(scenario)  // Deterministic based on seed
```

**CATCH doesn't need this** because:
- We observe real system behavior
- Reproducibility comes from trace replay
- No synthetic data generation

**Don't adopt this**.

## Recommendations

### High Priority: None
CATCH's current architecture is appropriate for its use case.

### Medium Priority

1. **Consider Entity Subscription Interfaces** (Optional)

   Instead of:
   ```go
   registry.RegisterEntity(entity, factory, move1, move2, move3)
   ```

   Could use:
   ```go
   type MoveSubscriber interface {
       SubscribesTo() []scorebooktypes.Move
   }

   func (tq *TaskQueue) SubscribesTo() []scorebooktypes.Move {
       return []scorebooktypes.Move{
           &moves.PollWorkflowTask{},
           &moves.AddWorkflowTask{},
       }
   }
   ```

2. **Separate Registry from Persistence**

   Clean separation like STAMP's ModelEnv:
   ```go
   type Registry struct {
       entities map[string]Entity
   }

   type Roster struct {
       registry    *Registry
       persistence *Persistence
   }
   ```

### Low Priority

1. **Scenario Path Exploration** (Future)

   For test ergonomics:
   ```go
   catch.Scenario(t, func(s *catch.Scenario) {
       if s.Maybe("fail-first-poll") {
           pitcher.Configure(...)
       }
       // Automatically runs multiple test variants
   })
   ```

### Don't Adopt

1. ❌ Action generators - CATCH observes real events
2. ❌ Reflection-based router - Current subscription model is simpler
3. ❌ Model Verify() - Keep verification in rulebook
4. ❌ Async/Future pattern - Not needed for event-based system
5. ❌ Generator context - No synthetic data in CATCH

## Summary

**STAMP** is excellent for *model-based testing* where you generate actions and verify model state.

**CATCH** is designed for *observability-based testing* where you observe real system behavior via OTEL traces.

The key insight: **CATCH doesn't need most of STAMP's complexity** because it operates at a different level. STAMP synthesizes behavior; CATCH observes behavior.

What we should keep from STAMP:
- Clean separation of concerns (already have it)
- Entity/model pattern (already have it)
- Property validation approach (already have it via rulebook)

What to consider:
- Interface-based move subscriptions (ergonomics)
- Registry/persistence separation (cleaner architecture)

What to avoid:
- Action generation (we observe real actions)
- Complex routing (subscriptions are simpler)
- Scenario generators (trace replay provides reproducibility)
