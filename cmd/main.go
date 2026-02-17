package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/yourusername/durable-execution-engine/engine"
	"github.com/yourusername/durable-execution-engine/examples/onboarding"
)

func main() {
	// Initialize engine with SQLite database
	eng, err := engine.NewEngine("./workflows.db")
	if err != nil {
		panic(err)
	}
	defer eng.Close()

	workflowID := "onboarding-employee-001"

	fmt.Println("=== Durable Execution Engine Demo ===")
	fmt.Printf("Workflow ID: %s\n\n", workflowID)

	// Setup crash simulation
	go func() {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("Press 'c' at any time to simulate a crash (exit)")
		for {
			char, _ := reader.ReadByte()
			if char == 'c' || char == 'C' {
				fmt.Println("\n💥 SIMULATING CRASH - Process terminating...")
				os.Exit(1)
			}
		}
	}()

	// Execute workflow (will resume if previously crashed)
	err = eng.Execute(workflowID, func(ctx *engine.Context) error {
		return onboarding.EmployeeOnboarding(ctx, "john.doe@example.com", "John Doe")
	})

	if err != nil {
		fmt.Printf("Workflow failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Workflow completed successfully!")
}
