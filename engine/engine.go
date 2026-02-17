package engine

import (
	"fmt"
)

// Engine is the main durable execution engine
type Engine struct {
	storage *Storage
}

// NewEngine creates a new durable execution engine
func NewEngine(dbPath string) (*Engine, error) {
	storage, err := NewStorage(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	return &Engine{
		storage: storage,
	}, nil
}

// Execute runs or resumes a workflow
// workflowID: unique identifier for this workflow instance
// workflowFn: the user's workflow function
func (e *Engine) Execute(workflowID string, workflowFn func(*Context) error) error {
	// Create workflow record if it doesn't exist
	if err := e.storage.CreateWorkflow(workflowID); err != nil {
		return fmt.Errorf("failed to create workflow: %w", err)
	}

	// Check if workflow is already completed
	status, err := e.storage.GetWorkflowStatus(workflowID)
	if err != nil {
		return fmt.Errorf("failed to get workflow status: %w", err)
	}

	if status == "completed" {
		fmt.Println("Workflow already completed")
		return nil
	}

	// Create context for the workflow
	ctx, err := newContext(workflowID, e.storage)
	if err != nil {
		return fmt.Errorf("failed to create context: %w", err)
	}

	// Execute the workflow function
	if err := workflowFn(ctx); err != nil {
		// Mark workflow as failed
		e.storage.UpdateWorkflowStatus(workflowID, "failed")
		return fmt.Errorf("workflow execution failed: %w", err)
	}

	// Mark workflow as completed
	if err := e.storage.UpdateWorkflowStatus(workflowID, "completed"); err != nil {
		return fmt.Errorf("failed to mark workflow as completed: %w", err)
	}

	return nil
}

// Close closes the engine and releases resources
func (e *Engine) Close() error {
	return e.storage.Close()
}

// GetWorkflowStatus returns the current status of a workflow
func (e *Engine) GetWorkflowStatus(workflowID string) (string, error) {
	return e.storage.GetWorkflowStatus(workflowID)
}
