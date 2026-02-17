package engine

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestBasicStepExecution(t *testing.T) {
	// Create temporary database
	dbPath := "./test_basic.db"
	defer os.Remove(dbPath)

	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	workflowID := "test-workflow-1"
	executionCount := 0

	err = eng.Execute(workflowID, func(ctx *Context) error {
		result, err := Step(ctx, "step-1", func() (string, error) {
			executionCount++
			return "result-1", nil
		})
		if err != nil {
			return err
		}

		if result != "result-1" {
			return fmt.Errorf("expected 'result-1', got '%s'", result)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}

	if executionCount != 1 {
		t.Errorf("expected step to execute once, executed %d times", executionCount)
	}

	// Verify workflow is marked as completed
	status, err := eng.GetWorkflowStatus(workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", status)
	}
}

func TestWorkflowResume(t *testing.T) {
	dbPath := "./test_resume.db"
	defer os.Remove(dbPath)

	workflowID := "test-workflow-resume"

	// First execution - simulate crash after first step
	step1Count := 0
	step2Count := 0

	eng1, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	err = eng1.Execute(workflowID, func(ctx *Context) error {
		_, err := Step(ctx, "step-1", func() (string, error) {
			step1Count++
			return "completed", nil
		})
		if err != nil {
			return err
		}

		// Simulate crash before step 2
		return errors.New("simulated crash")
	})

	if err == nil {
		t.Fatal("expected error from simulated crash")
	}

	eng1.Close()

	// Second execution - should resume and complete
	eng2, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng2.Close()

	err = eng2.Execute(workflowID, func(ctx *Context) error {
		_, err := Step(ctx, "step-1", func() (string, error) {
			step1Count++
			return "completed", nil
		})
		if err != nil {
			return err
		}

		_, err = Step(ctx, "step-2", func() (string, error) {
			step2Count++
			return "completed", nil
		})
		return err
	})

	if err != nil {
		t.Fatalf("workflow resume failed: %v", err)
	}

	// Step 1 should have executed only once (during first run)
	if step1Count != 1 {
		t.Errorf("expected step-1 to execute once, executed %d times", step1Count)
	}

	// Step 2 should have executed once (during second run)
	if step2Count != 1 {
		t.Errorf("expected step-2 to execute once, executed %d times", step2Count)
	}

	// Verify workflow is completed
	status, err := eng2.GetWorkflowStatus(workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", status)
	}
}

func TestConcurrentSteps(t *testing.T) {
	dbPath := "./test_concurrent.db"
	defer os.Remove(dbPath)

	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	workflowID := "test-workflow-concurrent"

	err = eng.Execute(workflowID, func(ctx *Context) error {
		// Launch concurrent steps
		ctx.Go(func() error {
			_, err := Step(ctx, "concurrent-step-1", func() (string, error) {
				time.Sleep(100 * time.Millisecond)
				return "result-1", nil
			})
			return err
		})

		ctx.Go(func() error {
			_, err := Step(ctx, "concurrent-step-2", func() (string, error) {
				time.Sleep(100 * time.Millisecond)
				return "result-2", nil
			})
			return err
		})

		ctx.Go(func() error {
			_, err := Step(ctx, "concurrent-step-3", func() (string, error) {
				time.Sleep(100 * time.Millisecond)
				return "result-3", nil
			})
			return err
		})

		// Wait for all to complete
		return ctx.Wait()
	})

	if err != nil {
		t.Fatalf("concurrent workflow failed: %v", err)
	}

	// Verify all steps completed
	status, err := eng.GetWorkflowStatus(workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", status)
	}
}

func TestIdempotency(t *testing.T) {
	dbPath := "./test_idempotent.db"
	defer os.Remove(dbPath)

	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	workflowID := "test-workflow-idempotent"
	executionCount := 0

	workflow := func(ctx *Context) error {
		_, err := Step(ctx, "idempotent-step", func() (int, error) {
			executionCount++
			return 42, nil
		})
		return err
	}

	// First execution
	err = eng.Execute(workflowID, workflow)
	if err != nil {
		t.Fatalf("first execution failed: %v", err)
	}

	firstCount := executionCount

	// Second execution (should not re-execute steps)
	err = eng.Execute(workflowID, workflow)
	if err != nil {
		t.Fatalf("second execution failed: %v", err)
	}

	// Step should have executed only during first run
	if executionCount != firstCount {
		t.Errorf("step re-executed on second run: %d vs %d", executionCount, firstCount)
	}
}

func TestErrorHandling(t *testing.T) {
	dbPath := "./test_error.db"
	defer os.Remove(dbPath)

	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	workflowID := "test-workflow-error"

	err = eng.Execute(workflowID, func(ctx *Context) error {
		_, err := Step(ctx, "failing-step", func() (string, error) {
			return "", errors.New("intentional failure")
		})
		return err
	})

	if err == nil {
		t.Fatal("expected error from failing step")
	}

	// Verify workflow is marked as failed
	status, err := eng.GetWorkflowStatus(workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", status)
	}
}

func TestComplexDataTypes(t *testing.T) {
	dbPath := "./test_complex.db"
	defer os.Remove(dbPath)

	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	workflowID := "test-workflow-complex"

	type User struct {
		ID    int
		Name  string
		Email string
	}

	err = eng.Execute(workflowID, func(ctx *Context) error {
		user, err := Step(ctx, "create-user", func() (User, error) {
			return User{
				ID:    123,
				Name:  "Test User",
				Email: "test@example.com",
			}, nil
		})
		if err != nil {
			return err
		}

		if user.ID != 123 || user.Name != "Test User" {
			return fmt.Errorf("unexpected user data: %+v", user)
		}

		// Test slice type
		_, err = Step(ctx, "get-tags", func() ([]string, error) {
			return []string{"tag1", "tag2", "tag3"}, nil
		})
		if err != nil {
			return err
		}

		// Test map type
		_, err = Step(ctx, "get-metadata", func() (map[string]interface{}, error) {
			return map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			}, nil
		})
		return err
	})

	if err != nil {
		t.Fatalf("complex types workflow failed: %v", err)
	}
}

func TestLoopSequencing(t *testing.T) {
	dbPath := "./test_loop.db"
	defer os.Remove(dbPath)

	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	workflowID := "test-workflow-loop"
	executionCounts := make(map[string]int)

	err = eng.Execute(workflowID, func(ctx *Context) error {
		for i := 0; i < 3; i++ {
			stepID := fmt.Sprintf("loop-step-%d", i)
			_, err := Step(ctx, stepID, func() (int, error) {
				executionCounts[stepID]++
				return i, nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("loop workflow failed: %v", err)
	}

	// Each iteration should have executed exactly once
	for i := 0; i < 3; i++ {
		stepID := fmt.Sprintf("loop-step-%d", i)
		if count, ok := executionCounts[stepID]; !ok || count != 1 {
			t.Errorf("step %s executed %d times, expected 1", stepID, count)
		}
	}
}
