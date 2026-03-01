# CATCH - Complete Testing Framework

CATCH is Temporal's complete testing framework that brings together observability, property validation, and fault injection into a unified system.

## Components

**CATCH** integrates:
- **Scout**: OTEL-based event instrumentation for observability
- **Catcher**: Receives and processes OTEL traces
- **Umpire**: Validates temporal properties and invariants
- **Pitcher**: Fault injection for chaos testing

## Quick Start

### For FunctionalTestBase

The CATCH system is automatically initialized in `FunctionalTestBase`. No setup required!

```go
type MyTestSuite struct {
    testcore.FunctionalTestBase
}

func (s *MyTestSuite) TestMyFeature() {
    // CATCH is already set up and monitoring

    // Your test code here...

    // CATCH automatically validates properties in TearDownTest
}
```

### Manual Setup

For custom test suites:

```go
import "go.temporal.io/server/common/testing/catch"

// Setup
c, err := catch.New(catch.Config{
    Logger:        logger,
    EnableScout:   true,  // OTEL instrumentation
    EnableUmpire:  true,  // Property validation
    EnablePitcher: true,  // Fault injection
})

// Add span exporter to test cluster config
testClusterConfig.SpanExporters = c.GetSpanExporters()

// Teardown
violations := c.Check(ctx)
require.Empty(t, violations)
c.Shutdown(ctx)
```

## Instrumentation API

### Basic Instrumentation

```go
import (
    "go.temporal.io/server/common/testing/catch"
    "go.opentelemetry.io/otel/attribute"
)

// Create span with duration
ctx, span := catch.Instrument(ctx, "operation.name",
    catch.WorkflowID(workflowID),
    catch.NamespaceID(namespaceID),
    attribute.String("custom.field", "value"),
)
defer span.End()

// Record point-in-time event
catch.RecordEvent(ctx, "event.name",
    catch.WorkflowID(workflowID),
    attribute.String("status", "completed"),
)

// Record errors
if err != nil {
    catch.RecordError(ctx, err,
        catch.WorkflowID(workflowID),
    )
}
```

### Helper Functions

CATCH provides helpers for common attributes:

```go
catch.WorkflowID(id string)       // Workflow ID
catch.RunID(id string)            // Run ID
catch.NamespaceID(id string)      // Namespace ID
catch.Namespace(name string)      // Namespace name
catch.TaskQueue(name string)      // Task queue name
catch.ShardID(id int32)           // Shard ID
catch.Operation(name string)      // Operation name
catch.LockType(lockType string)   // Lock type
catch.ResourceType(typ string)    // Resource type
```

### Example: Workflow Lock Instrumentation

```go
// Lock acquisition
ctx, span := catch.Instrument(ctx, "workflow.cache.lock.acquire",
    catch.WorkflowID(wfKey.WorkflowID),
    catch.RunID(wfKey.RunID),
    catch.NamespaceID(wfKey.NamespaceID),
    catch.LockType("high"),
)
defer span.End()

// Lock release
catch.RecordEvent(ctx, "workflow.cache.lock.released",
    catch.WorkflowID(wfKey.WorkflowID),
    catch.RunID(wfKey.RunID),
    catch.NamespaceID(wfKey.NamespaceID),
    attribute.String("release.reason", "normal"),
)
```

## Property Validation

The Umpire component validates temporal properties automatically:

```go
// Properties are checked automatically in FunctionalTestBase.TearDownTest()

// Manual check:
violations := s.GetCatch().Check(ctx)
if len(violations) > 0 {
    for _, v := range violations {
        t.Logf("Violation: %v", v)
    }
}
```

## Fault Injection

Configure fault injection with Pitcher:

```go
import "go.temporal.io/server/tools/catch/pitcher"

// Configure in test
s.ConfigurePitcher(&matchingservice.AddWorkflowTaskRequest{}, pitcher.PlayConfig{
    Action: pitcher.ActionError,
    Probability: 0.5,  // 50% chance
    ErrorCode: codes.ResourceExhausted,
})

// Reset between tests (automatic in FunctionalTestBase)
s.GetCatch().Reset()
```

## Architecture

```
Application Code
    ↓ catch.Instrument(...)
OTEL Tracer
    ↓ Spans/Events
Scout SpanExporter
    ↓ ptrace.Traces
Catcher (TraceHandler)
    ↓ Forward to processors
Umpire → Property Validation
```

## gRPC Interceptor

CATCH provides a gRPC interceptor for automatic instrumentation:

```go
import "go.temporal.io/server/common/testing/catch"

// Automatically included in FunctionalTestBase
additionalInterceptors := []grpc.UnaryServerInterceptor{
    catch.UnaryServerInterceptor(),
}
```

The interceptor:
- Records gRPC moves for Umpire validation
- Injects faults via Pitcher
- Tracks all RPC calls automatically

## Test Lifecycle

### Setup (Automatic in FunctionalTestBase)

```go
func (s *FunctionalTestBase) SetupSuiteWithCluster() {
    // 1. Initialize CATCH
    catch, err := catch.New(catch.Config{...})

    // 2. Add CATCH interceptor
    interceptors = append(interceptors, catch.UnaryServerInterceptor())

    // 3. Register span exporter
    testClusterConfig.SpanExporters = catch.GetSpanExporters()
}
```

### Teardown (Automatic in FunctionalTestBase)

```go
func (s *FunctionalTestBase) TearDownTest() {
    // 1. Reset CATCH state
    s.catch.Reset()

    // 2. Export OTEL traces
    s.exportOTELTraces()

    // 3. Check for violations
    s.checkWatchdog()  // Calls s.catch.Check()
}
```

## When to Use CATCH Instrumentation

**Instrument these events:**
- Critical synchronization points (locks, barriers)
- State transitions affecting correctness
- Resource acquisition/release
- Operations that Umpire properties need to validate

**Don't instrument:**
- High-frequency operations (per-task/per-event)
- Operations that don't affect correctness
- General performance tracing (use regular OTEL)

## Best Practices

1. **Use helper functions** for common attributes:
   ```go
   // Good
   catch.WorkflowID(id)

   // Avoid
   attribute.String(catch.AttrWorkflowID, id)
   ```

2. **Keep event names consistent**:
   ```go
   // Good
   "workflow.cache.lock.acquire"
   "workflow.cache.lock.released"

   // Avoid
   "AcquireLock"
   "lock_release"
   ```

3. **Include release reason** for cleanup operations:
   ```go
   catch.RecordEvent(ctx, "workflow.cache.lock.released",
       catch.WorkflowID(id),
       attribute.String("release.reason", "normal"), // or "error", "panic"
   )
   ```

4. **Record errors with context**:
   ```go
   if err != nil {
       catch.RecordError(ctx, err,
           catch.WorkflowID(id),
           catch.Operation("AddWorkflowTask"),
       )
       return err
   }
   ```

## Examples

See:
- `service/history/workflow/cache/cache.go` - Workflow lock instrumentation
- `tests/testcore/functional_test_base.go` - Complete setup example
- `common/testing/catch/testbase.go` - TestBase helper

## API Reference

### Main Types

- `catch.Catch` - Main CATCH system
- `catch.Config` - Configuration
- `catch.TestBase` - Helper for embedding in test suites

### Core Functions

- `catch.New(cfg)` - Create CATCH system
- `catch.Instrument(ctx, name, attrs...)` - Create span
- `catch.RecordEvent(ctx, name, attrs...)` - Record event
- `catch.RecordError(ctx, err, attrs...)` - Record error

### CATCH Methods

- `c.Check(ctx)` - Validate properties
- `c.Reset()` - Clear state between tests
- `c.Shutdown(ctx)` - Clean shutdown
- `c.GetSpanExporters()` - Get exporters for test cluster
- `c.Umpire()` - Access Umpire
- `c.Pitcher()` - Access Pitcher
- `c.Catcher()` - Access Catcher

## Troubleshooting

### No traces captured

Ensure span exporter is registered:
```go
testClusterConfig.SpanExporters = catch.GetSpanExporters()
```

### Violations not detected

Check Umpire is enabled:
```go
catch.New(catch.Config{
    EnableUmpire: true,  // ← Must be true
})
```

### Faults not injected

Check Pitcher is enabled and configured:
```go
catch.New(catch.Config{
    EnablePitcher: true,  // ← Must be true
})
s.ConfigurePitcher(targetType, config)
```

## Further Reading

- [PLAN.md](../../../PLAN.md) - Overall CATCH architecture
- [tools/catch/catcher](../../../tools/catch/catcher) - OTEL receiver
- [tools/catch/umpire](../../../tools/catch/umpire) - Property validator
- [tools/catch/pitcher](../../../tools/catch/pitcher) - Fault injector
