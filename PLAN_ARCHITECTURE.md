# CATCH Architecture

## Baseball Metaphor

| Term | Baseball | CATCH System |
|------|----------|--------------|
| **Play** | Atomic action (bunt, steal, etc.) | Atomic action (delay, fail, cancel, timeout) |
| **Pitcher** | Makes plays | Executes plays (fault injection engine) |
| **Game** | Series of plays | Test scenario with configured plays and matching criteria |
| **Scorebook** | Records what happened | Records plays that were made (importer) |
| **Umpire** | Validates outcomes | Validates properties (expected behavior) |
| **Roster** | Team members | Entities (Workflow, TaskQueue, WorkflowTask) |
| **Rulebook** | Rules of the game | Property models (invariants) |
| **Skipper** | Team manager | Intelligent test orchestration |
| **Scout** | Reports game events | OTEL span exporter that feeds umpire |

## Core Concepts

### Play (Atomic Action)
**Location**: `tools/catch/play/play.go`

A Play is an atomic action that:
- Can be initiated by a user or pitcher
- Can be recorded by the scorebook
- Describes only the INTENTION (delay, fail, cancel, timeout)
- Has NO knowledge of specific services (Matching, History, etc.)
- Has NO expected outcomes (that's Umpire's job)

```go
type Play struct {
    Type      PlayType         // delay, fail, cancel, timeout, drop
    Params    map[string]any   // Action-specific parameters
    Timestamp time.Time        // When play was made
    Target    string           // Set at runtime (for scorebook)
}

// Play types
const (
    PlayDelay   PlayType = "delay"
    PlayFail    PlayType = "fail"
    PlayCancel  PlayType = "cancel"
    PlayTimeout PlayType = "timeout"
    PlayDrop    PlayType = "drop"
)
```

### Pitcher (Makes Plays)
**Location**: `tools/catch/pitcher/pitcher.go`

The Pitcher:
- Makes plays based on matching criteria
- Returns the play that was made (for scorebook recording)
- Uses typed proto message references for targets
- Each play matches ONCE and is deleted

```go
type Pitcher interface {
    // MakePlay executes a play if matching criteria are met
    // Returns the play that was made and error to inject
    MakePlay(ctx context.Context, targetType any, request any) (*play.Play, error)

    // Configure sets play configuration with matching criteria
    // Game provides the matching criteria
    Configure(targetType any, config PlayConfig)

    Reset()
}

type PlayConfig struct {
    Play  play.Play       // The action to take
    Match *MatchCriteria  // When to make this play (Game-level knowledge)
}

type MatchCriteria struct {
    // RPC target (Game configures this, not Play/Pitcher)
    Target string // e.g., "matchingservice.AddWorkflowTask"

    // Entity criteria
    WorkflowID  string
    NamespaceID string
    TaskQueue   string
    RunID       string
    Custom      map[string]any
}
```

### Game (Test Scenario)
**Location**: `tools/catch/play/game.go`

Game is where knowledge of specific services lives:
- Configures which plays to make where
- Specifies RPC targets and matching criteria
- Defines test scenarios without expected outcomes

```go
type Game struct {
    Name        string        // Test scenario name
    Description string        // What this tests
    PlayConfigs []PlayConfig  // Which plays to make and when
    Tags        []string      // Categorization
}

type PlayConfig struct {
    Play        play.Play      // Action to take
    Match       MatchCriteria  // When to make this play
    Description string         // What this tests
}
```

### Example: Matching Failure Game

```go
game := &Game{
    Name: "MatchingFailureRetry",
    Description: "Transient matching failure should not prevent workflow completion",
    PlayConfigs: []PlayConfig{
        {
            Play: play.FailPlay(pitcher.ErrorResourceExhausted),
            Match: MatchCriteria{
                // Game specifies the RPC target
                Target: "matchingservice.AddWorkflowTask",
                // Match first request (no entity criteria = match any)
            },
            Description: "Fail first task addition with RESOURCE_EXHAUSTED",
        },
    },
    Tags: []string{"resilience", "matching", "retry"},
}
```

### Roster (Entities and Relationships)
**Location**: `tools/catch/roster/`

Roster defines the entities in the system:
- **Workflow**: Workflow instances with FSM state
- **TaskQueue**: Task queue entities
- **WorkflowTask**: Workflow task entities
- **Identity**: Entity identification system

Entities track state and relationships. Roster provides the entity registry.

### Rulebook (Property Models)
**Location**: `tools/catch/rulebook/`

Rulebook contains property validation models (invariants):
- `StuckWorkflowModel`: Detects workflows that don't complete
- `LostTaskModel`: Detects tasks that aren't polled
- `WorkflowLifecycleInvariants`: Validates workflow state transitions
- `TaskDeliveryGuarantees`: Validates task delivery
- `RetryResilienceModel`: Validates retry behavior

Models query the roster to validate properties.

### Scorebook (Records Game Moves)
**Location**: `tools/catch/scorebook/`

Scorebook:
- Imports OTLP spans and converts to moves (events that happened)
- Records what plays were made during the game
- Stores entity state transitions
- Provides moves to roster for entity updates

**Moves** (`scorebook/moves/`): Domain events like StartWorkflow, AddWorkflowTask, PollWorkflowTask, etc.

### Umpire (Orchestrates Validation)
**Location**: `tools/catch/umpire/`

Umpire:
- Receives OTLP traces via scorebook
- Routes events to roster entities
- Runs rulebook models to check for violations
- Reports violations
- Does NOT define plays or expected outcomes

### Scout (Reports to Umpire)
**Location**: `tools/catch/scout/`

Scout:
- OTEL span exporter that forwards spans to umpire
- Integrates with existing OTEL instrumentation
- Converts SDK spans to OTLP format
- Seamlessly feeds telemetry to umpire for validation

### Skipper (Test Orchestration)
**Location**: `tools/catch/skipper/`

Skipper:
- Selects which games to run
- Tracks coverage of test scenarios
- Uses strategies (random, coverage-driven, sequential, weighted)
- Provides feedback for adaptive test generation

## Module Structure

```
tools/catch/
├── play/              # Play and Game definitions
│   ├── play.go       # Atomic actions (delay, fail, timeout, etc.)
│   └── game.go       # Test scenarios with PlayConfigs
├── pitcher/           # Makes plays
│   ├── pitcher.go    # Pitcher interface with MakePlay()
│   └── interceptor.go # gRPC interceptor
├── roster/            # Entities and relationships
│   ├── registry.go   # Entity registry
│   ├── workflow.go   # Workflow entity
│   ├── task_queue.go # TaskQueue entity
│   └── identity/     # Entity identification
├── rulebook/          # Property models
│   ├── registry.go   # Model registry
│   ├── stuck_workflow.go
│   ├── lost_task.go
│   └── ...          # Other models
├── scorebook/         # Records game moves
│   ├── importer.go   # OTLP span -> move importer
│   └── moves/        # Move definitions (events that happened)
├── scout/             # Reports to umpire
│   └── exporter.go   # OTEL span exporter
├── skipper/           # Test orchestration
│   └── skipper.go    # Game selection strategies
└── umpire/            # Orchestration
    ├── umpire.go     # Main umpire
    ├── model.go      # Model dependencies
    └── violation.go  # Violation reporting
```

## Data Flow

```
┌─────────────────────────────────────────────────────────┐
│                        Test                              │
│  ┌──────────┐                                            │
│  │  Game    │ Configures plays with matching criteria   │
│  │(Skipper) │ Selects which games to run                │
│  └────┬─────┘                                            │
│       │                                                   │
│       v                                                   │
│  ┌──────────┐         ┌──────────┐                      │
│  │ Pitcher  │─makes─> │   Play   │                      │
│  └────┬─────┘         └──────────┘                      │
│       │                                                   │
│       v                                                   │
│  ┌──────────┐                                            │
│  │  gRPC    │ Interceptor calls MakePlay()               │
│  │Intercept │                                            │
│  └────┬─────┘                                            │
│       │                                                   │
│       v                                                   │
│  ┌──────────┐                                            │
│  │  OTLP    │ Telemetry spans                           │
│  │  Traces  │                                            │
│  └────┬─────┘                                            │
│       │                                                   │
│       v                                                   │
│  ┌──────────┐         ┌──────────┐      ┌──────────┐   │
│  │Scorebook │─event─> │  Roster  │ <─── │ Rulebook │   │
│  │(Importer)│         │(Entities)│ ───> │ (Models) │   │
│  └──────────┘         └──────────┘      └────┬─────┘   │
│                                                │          │
│                                                v          │
│                                           ┌──────────┐   │
│                                           │ Umpire   │   │
│                                           │(Validate)│   │
│                                           └──────────┘   │
└─────────────────────────────────────────────────────────┘
```

## Key Design Principles

1. **Separation of Concerns**
   - Play = what to do (delay, fail)
   - Game = when/where to do it (matching criteria)
   - Umpire = is it correct? (property validation)

2. **Service Agnostic**
   - Play/Pitcher don't know about Matching, History, Frontend
   - Game provides that knowledge via MatchCriteria

3. **Observable Actions**
   - Every play made can be recorded by scorebook
   - Scorebook records are inputs to umpire validation

4. **One-Time Execution**
   - Each play matches once and is deleted
   - Multiple matches require multiple play configurations

5. **Typed Proto References**
   - Use `&matchingservice.AddWorkflowTaskRequest{}` instead of strings
   - Type-safe target identification via reflection

## Usage Example

```go
// Setup pitcher in test
pitcher := pitcher.New()
pitcher.Set(pitcher)

// Configure game-level play
pitcher.Configure(
    &matchingservice.AddWorkflowTaskRequest{},
    pitcher.PlayConfig{
        Play: play.FailPlay(pitcher.ErrorResourceExhausted),
        Match: &pitcher.MatchCriteria{
            // Game specifies when/where
            WorkflowID: "test-workflow-123",
        },
    },
)

// Run test - interceptor automatically makes plays
// Scorebook records plays
// Umpire validates properties
```

## Migration Notes

**Old Design** (incorrect):
- Play = test scenario with expected outcomes
- Pitch = fault injection for specific RPC
- Play contained RPC targets

**New Design** (correct):
- Play = atomic action (delay, fail, etc.)
- Game = test scenario with play configurations
- Pitcher = makes plays
- Umpire = validates outcomes
- RPC targets specified in Game's MatchCriteria
