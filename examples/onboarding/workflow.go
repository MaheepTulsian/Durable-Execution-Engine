package onboarding

import (
	"fmt"
	"time"

	"github.com/yourusername/durable-execution-engine/engine"
)

// EmployeeOnboarding implements the required 4-step workflow
func EmployeeOnboarding(ctx *engine.Context, email string, employeeName string) error {
	fmt.Println("Starting employee onboarding workflow...")

	// Step 1: Create Record (Sequential)
	userID, err := engine.Step(ctx, "create-user-record", func() (int, error) {
		fmt.Printf("Creating user record for %s...\n", email)
		time.Sleep(1 * time.Second) // Simulate API call
		return 12345, nil
	})
	if err != nil {
		return fmt.Errorf("failed to create user record: %w", err)
	}

	fmt.Printf("User created with ID: %d\n", userID)

	// Step 2 & 3: Provision Laptop & Access (Parallel)
	ctx.Go(func() error {
		_, err := engine.Step(ctx, "provision-laptop", func() (string, error) {
			fmt.Println("Provisioning laptop...")
			time.Sleep(2 * time.Second)
			return "LAPTOP-001", nil
		})
		return err
	})

	ctx.Go(func() error {
		_, err := engine.Step(ctx, "provision-access", func() (string, error) {
			fmt.Println("Provisioning system access...")
			time.Sleep(2 * time.Second)
			return "ACCESS-GRANTED", nil
		})
		return err
	})

	// Wait for parallel steps to complete
	if err := ctx.Wait(); err != nil {
		return fmt.Errorf("parallel provisioning failed: %w", err)
	}

	fmt.Println("Parallel provisioning completed")

	// Step 4: Send Welcome Email (Sequential)
	_, err = engine.Step(ctx, "send-welcome-email", func() (string, error) {
		fmt.Printf("Sending welcome email to %s...\n", email)
		time.Sleep(1 * time.Second)
		return "EMAIL-SENT", nil
	})
	if err != nil {
		return fmt.Errorf("failed to send welcome email: %w", err)
	}

	return nil
}
