package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/yourusername/durable-execution-engine/engine"
	"github.com/yourusername/durable-execution-engine/examples"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/advanced_demo.go [pipeline|order]")
		os.Exit(1)
	}

	// Initialize engine
	eng, err := engine.NewEngine("./advanced-workflows.db")
	if err != nil {
		panic(err)
	}
	defer eng.Close()

	// Setup crash simulation
	go func() {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("\nPress 'c' at any time to simulate a crash (exit)\n")
		for {
			char, _ := reader.ReadByte()
			if char == 'c' || char == 'C' {
				fmt.Println("\n💥 SIMULATING CRASH - Process terminating...")
				os.Exit(1)
			}
		}
	}()

	mode := os.Args[1]

	switch mode {
	case "pipeline":
		fmt.Println("=== Data Processing Pipeline Demo ===")
		workflowID := "data-pipeline-001"
		fmt.Printf("Workflow ID: %s\n\n", workflowID)

		dataFiles := []string{"data1.csv", "data2.csv", "data3.csv", "data4.csv"}

		err = eng.Execute(workflowID, func(ctx *engine.Context) error {
			return examples.DataProcessingPipeline(ctx, dataFiles)
		})

	case "order":
		fmt.Println("=== Order Fulfillment Demo ===")
		workflowID := "order-001"
		fmt.Printf("Workflow ID: %s\n\n", workflowID)

		items := []string{"Widget A", "Gadget B", "Doohickey C"}

		err = eng.Execute(workflowID, func(ctx *engine.Context) error {
			return examples.OrderFulfillment(ctx, "ORDER-12345", items)
		})

	default:
		fmt.Printf("Unknown mode: %s\n", mode)
		fmt.Println("Usage: go run cmd/advanced_demo.go [pipeline|order]")
		os.Exit(1)
	}

	if err != nil {
		fmt.Printf("Workflow failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Workflow completed successfully!")
}
