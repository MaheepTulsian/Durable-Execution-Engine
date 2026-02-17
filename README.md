# Durable Execution Engine

A fault-tolerant workflow execution library for Go that makes normal code automatically resumable and crash-resistant.

**Inspired by**: Temporal, Cadence, DBOS, and Azure Durable Functions

---

## 📋 Table of Contents

- [Quick Start](#quick-start)
- [What's Inside](#whats-inside)
- [How It Works](#how-it-works)
  - [Sequence Tracking & Loops](#sequence-tracking--loops)
  - [Thread Safety & Parallel Execution](#thread-safety--parallel-execution)
- [Running the Demo](#running-the-demo)
- [API Reference](#api-reference)
- [Architecture](#architecture)
- [Design Decisions](#design-decisions)
- [Testing](#testing)
- [Limitations](#limitations)

---

## Quick Start

### Installation

```bash
go get github.com/MaheepTulsian/durable-execution-engine
```

### Basic Example

```go
package main

import (
    "fmt"
    "github.com/MaheepTulsian/durable-execution-engine/engine"
)

func main() {
    eng, _ := engine.NewEngine("./workflows.db")
    defer eng.Close()

    eng.Execute("my-workflow", func(ctx *engine.Context) error {
        // Steps execute once and are memoized
        userID, _ := engine.Step(ctx, "create-user", func() (int, error) {
            fmt.Println("Creating user...")
            return 12345, nil
        })

        _, err := engine.Step(ctx, "send-email", func() (string, error) {
            fmt.Printf("Emailing user %d...\n", userID)
            return "SENT", nil
        })
        return err
    })
}
```

### Parallel Execution

```go
eng.Execute("parallel-workflow", func(ctx *engine.Context) error {
    ctx.Go(func() error {
        _, err := engine.Step(ctx, "task-1", func() (string, error) {
            return doTask1(), nil
        })
        return err
    })

    ctx.Go(func() error {
        _, err := engine.Step(ctx, "task-2", func() (string, error) {
            return doTask2(), nil
        })
        return err
    })

    return ctx.Wait() // Waits for all parallel steps
})
```

---

## What's Inside

This repository contains:

| Component | Description |
|-----------|-------------|
| **`engine/`** | Core library: engine, context, storage, sequence tracking |
| **`examples/onboarding/`** | Sample workflow: Employee onboarding with sequential & parallel steps |
| **`cmd/main.go`** | CLI demo with crash simulation (press 'c' to crash) |
| **`README.md`** | This file |
| **`prompts.txt`** | All AI prompts used during development |

### Features

✅ **Crash Recovery** - Resume from exact failure point
✅ **Step Memoization** - Completed steps never re-execute
✅ **Type-Safe Generics** - `Step[T any]()` supports any serializable type
✅ **Parallel Execution** - Thread-safe concurrent steps with `errgroup`
✅ **SQLite Storage** - WAL mode for reliability and concurrency

---

## How It Works

### Sequence Tracking & Loops

**The Problem**: How do we track steps uniquely, especially in loops, and ensure they don't re-execute on crash recovery?

**The Solution**: Each step gets a composite key: `stepID:sequenceNum`

```go
// Step key format: "stepID:sequenceNum"
create-user:1  // First step
send-email:2   // Second step
```

**For loops**, users must provide unique step IDs per iteration:

```go
for i := 0; i < 3; i++ {
    stepID := fmt.Sprintf("process-%d", i)
    engine.Step(ctx, stepID, func() (string, error) {
        return processItem(i)
    })
}
// Creates: process-0:1, process-1:2, process-2:3
```

**On crash recovery**:
1. We load a persistent mapping: `stepID → sequenceNum` from the database
2. When `Step(ctx, "process-0", ...)` is called again, we check: "Have we seen this step ID?"
3. If yes → reuse the same sequence number → check database → skip if completed
4. If no → assign new sequence number → execute

**Result**: Deterministic replay - the same step always gets the same sequence number across restarts.

**Example Execution**:
```
First run:
  create-user:1   ✓ completed
  process-0:2     ✓ completed
  process-1:3     ✗ CRASH

Second run:
  create-user:1   ⏭ skipped (already completed)
  process-0:2     ⏭ skipped (already completed)
  process-1:3     ▶ executes (was in progress)
  process-2:4     ▶ executes (new)
```

---

### Thread Safety & Parallel Execution

**The Problem**: Multiple goroutines executing steps concurrently could cause race conditions.

**The Solution**: Multi-layered safety approach

#### 1. Mutex Protection

```go
ctx.mu.Lock()
seqNum, exists := ctx.stepIDToSeq[id]
if !exists {
    seqNum = atomic.AddInt64(&ctx.sequenceNum, 1)
    ctx.stepIDToSeq[id] = seqNum
}
ctx.mu.Unlock()
```

Protects the `stepIDToSeq` mapping from concurrent access.

#### 2. Atomic Operations

```go
seqNum := atomic.AddInt64(&ctx.sequenceNum, 1)
```

Guarantees unique sequence numbers without races.

#### 3. Database Safety

```go
PRAGMA journal_mode=WAL      // Concurrent reads during writes
PRAGMA busy_timeout=5000     // Wait for locks instead of failing
db.SetMaxOpenConns(1)        // SQLite single-writer limitation
```

#### 4. Retry Logic

```go
func retryOnBusy(fn func() error) error {
    for i := 0; i < 5; i++ {
        if err := fn(); err == nil || !isSQLiteBusy(err) {
            return err
        }
        time.Sleep(10ms * (i+1)) // Exponential backoff
    }
}
```

Handles database contention gracefully.

#### 5. Error Propagation

Uses `errgroup.Group` for safe concurrent execution:
- Automatic error collection
- Context cancellation on first error
- Safe waiting for all goroutines

**Safety Guarantees**:
- ✅ No race conditions (mutex + atomic)
- ✅ Unique sequence numbers
- ✅ No database corruption (WAL + ACID)
- ✅ Proper error handling (errgroup)

---

## Running the Demo

```bash
# Start the workflow
go run cmd/main.go

# Press 'c' to simulate crash
💥 SIMULATING CRASH - Process terminating...

# Restart - it resumes!
go run cmd/main.go
[SKIPPED] create-user-record (already completed)
[SKIPPED] provision-laptop (already completed)
✅ Workflow completed successfully!
```

---

## API Reference

### Engine

```go
engine.NewEngine(dbPath string) (*Engine, error)
engine.Execute(workflowID string, fn func(*Context) error) error
engine.Close() error
```

### Context

```go
// Execute a step with type-safe return value
engine.Step[T any](ctx *Context, id string, fn func() (T, error)) (T, error)

// Launch concurrent step
ctx.Go(fn func() error)

// Wait for all concurrent steps
ctx.Wait() error
```

### Example: Complete Workflow

```go
eng.Execute("order-workflow", func(ctx *engine.Context) error {
    // Sequential step
    order, _ := engine.Step(ctx, "create-order", func() (Order, error) {
        return createOrder()
    })

    // Parallel steps
    ctx.Go(func() error {
        _, err := engine.Step(ctx, "charge-card", func() (string, error) {
            return chargeCard(order.Total)
        })
        return err
    })

    ctx.Go(func() error {
        _, err := engine.Step(ctx, "reserve-inventory", func() (bool, error) {
            return reserveItems(order.Items)
        })
        return err
    })

    // Wait for parallel steps
    if err := ctx.Wait(); err != nil {
        return err
    }

    // Final step
    _, err := engine.Step(ctx, "ship-order", func() (string, error) {
        return shipOrder(order.ID)
    })
    return err
})
```

---

## Architecture

### Database Schema

```sql
CREATE TABLE workflows (
    workflow_id TEXT PRIMARY KEY,
    status TEXT NOT NULL  -- 'running', 'completed', 'failed'
);

CREATE TABLE steps (
    workflow_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    sequence_num INTEGER NOT NULL,
    step_key TEXT UNIQUE NOT NULL,  -- "stepID:sequenceNum"
    status TEXT NOT NULL,            -- 'in_progress', 'completed', 'failed'
    output BLOB,                     -- JSON-serialized result
    completed_at TIMESTAMP
);
```

### Core Components

**Engine** (`engine.go`)
- Manages workflow lifecycle
- Creates and persists workflow state

**Context** (`context.go`)
- Provides `Step()` primitive
- Manages sequence tracking
- Handles parallel execution

**Storage** (`storage.go`)
- SQLite persistence layer
- WAL mode for concurrency
- Retry logic for busy errors

**Sequence** (`sequence.go`)
- Generates unique step keys
- Maps step IDs to sequence numbers

---

## Design Decisions

### Step Key Format
**Choice**: `stepID:sequenceNum` (e.g., `create-user:1`)
**Why**: Simple, unique, efficient indexing

### Error Recovery
**Choice**: Fail workflow, preserve state, allow retry
**Why**: User controls retry logic, clear failure semantics

### Sequence Counter
**Choice**: In-memory, reconstructed from DB on startup
**Why**: Fast (no DB write per step), safe (max sequence from DB)

### Concurrency Model
**Choice**: `errgroup.Group`
**Why**: Battle-tested, built-in error handling, familiar to Go devs

### Database
**Choice**: SQLite with `modernc.org/sqlite` (pure Go)
**Why**: Zero external deps, ACID guarantees, no CGO required

---

## Testing

```bash
go test ./engine/... -v -cover
```

**Coverage: 71.3%** | **All tests passing** ✅

Tests cover:
- Basic step execution & memoization
- Crash recovery & resume
- Concurrent execution
- Idempotency
- Error handling
- Complex types (structs, slices, maps)
- Loop sequencing

---

## Limitations

1. **Determinism Required**: Steps must be deterministic (same inputs → same outputs)
2. **No Input Validation**: Changing workflow inputs between retries is undefined behavior
3. **External Side Effects**: Only return values are memoized, not external effects (DB writes, API calls)
4. **Single Machine**: Not designed for distributed execution

---

## Contributing

This is a learning project. Feel free to fork and experiment!

## License

MIT

---

**Built with Go 1.18+** | Inspired by [Temporal](https://temporal.io/), [Cadence](https://cadenceworkflow.io/), [DBOS](https://www.dbos.dev/), [Azure Durable Functions](https://learn.microsoft.com/en-us/azure/azure-functions/durable/)
