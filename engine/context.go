package engine

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

// Context represents the execution context for a workflow
type Context struct {
	WorkflowID     string
	sequenceNum    int64
	storage        *Storage
	completedSteps map[string][]byte
	stepIDToSeq    map[string]int64 // Maps step ID to its sequence number
	mu             sync.Mutex
	eg             *errgroup.Group
}

// newContext creates a new workflow context
func newContext(workflowID string, storage *Storage) (*Context, error) {
	// Load completed steps from database
	completedSteps, err := storage.LoadCompletedSteps(workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to load completed steps: %w", err)
	}

	// Load step ID to sequence number mapping
	stepIDToSeq, err := storage.LoadStepIDMapping(workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to load step ID mapping: %w", err)
	}

	// Get the maximum sequence number to resume from
	maxSeq, err := storage.GetMaxSequenceNum(workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get max sequence: %w", err)
	}

	eg := &errgroup.Group{}

	return &Context{
		WorkflowID:     workflowID,
		sequenceNum:    maxSeq,
		storage:        storage,
		completedSteps: completedSteps,
		stepIDToSeq:    stepIDToSeq,
		eg:             eg,
	}, nil
}

// Step is the core primitive - executes a function with memoization
// Generic type T for any return type
// id: user-provided step identifier (e.g., "create-user", "send-email")
// fn: the function to execute (only runs if not already completed)
func Step[T any](ctx *Context, id string, fn func() (T, error)) (T, error) {
	var zero T

	// 1. Check if we've seen this step ID before, reuse sequence if so
	ctx.mu.Lock()
	seqNum, exists := ctx.stepIDToSeq[id]
	if !exists {
		// Generate new sequence number
		seqNum = atomic.AddInt64(&ctx.sequenceNum, 1)
		ctx.stepIDToSeq[id] = seqNum
	}
	ctx.mu.Unlock()

	stepKey := generateStepKey(id, seqNum)

	// 2. Check in-memory cache first
	ctx.mu.Lock()
	cached, ok := ctx.completedSteps[stepKey]
	ctx.mu.Unlock()

	if ok {
		var result T
		if err := json.Unmarshal(cached, &result); err != nil {
			return zero, fmt.Errorf("failed to unmarshal cached result: %w", err)
		}
		fmt.Printf("[SKIPPED] %s (already completed)\n", id)
		return result, nil
	}

	// 3. Check database
	output, found, err := ctx.storage.GetStep(ctx.WorkflowID, stepKey)
	if err != nil {
		return zero, fmt.Errorf("failed to check step in database: %w", err)
	}

	if found {
		var result T
		if err := json.Unmarshal(output, &result); err != nil {
			return zero, fmt.Errorf("failed to unmarshal database result: %w", err)
		}

		// Cache in memory
		ctx.mu.Lock()
		ctx.completedSteps[stepKey] = output
		ctx.mu.Unlock()

		fmt.Printf("[SKIPPED] %s (already completed)\n", id)
		return result, nil
	}

	// 4. Mark as in-progress (zombie protection)
	if err := ctx.storage.MarkStepInProgress(ctx.WorkflowID, stepKey, id, seqNum); err != nil {
		return zero, fmt.Errorf("failed to mark step in progress: %w", err)
	}

	// 5. Execute the function
	result, err := fn()
	if err != nil {
		// Save error to database
		ctx.storage.SaveStepError(ctx.WorkflowID, stepKey, err.Error())
		return zero, err
	}

	// 6. Serialize and save
	output, err = json.Marshal(result)
	if err != nil {
		return zero, fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := ctx.storage.SaveStep(ctx.WorkflowID, stepKey, output); err != nil {
		return zero, fmt.Errorf("failed to save step: %w", err)
	}

	// Cache in memory
	ctx.mu.Lock()
	ctx.completedSteps[stepKey] = output
	ctx.mu.Unlock()

	return result, nil
}

// Go runs a function concurrently (like errgroup)
func (ctx *Context) Go(fn func() error) {
	ctx.eg.Go(fn)
}

// Wait waits for all concurrent operations to complete
func (ctx *Context) Wait() error {
	return ctx.eg.Wait()
}

// AutoStep is a bonus feature that automatically generates step IDs from the call location
func AutoStep[T any](ctx *Context, fn func() (T, error)) (T, error) {
	// Get caller location (skip 1 frame to get the actual caller)
	autoID := getCallerLocation(2)
	return Step(ctx, autoID, fn)
}
