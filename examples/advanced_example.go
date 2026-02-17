package examples

import (
	"fmt"
	"time"

	"github.com/yourusername/durable-execution-engine/engine"
)

// DataProcessingPipeline demonstrates advanced features:
// - Loops with dynamic step IDs
// - Complex data types
// - Error handling
// - Mix of sequential and parallel steps
func DataProcessingPipeline(ctx *engine.Context, dataFiles []string) error {
	fmt.Println("Starting data processing pipeline...")

	// Step 1: Initialize pipeline
	pipelineID, err := engine.Step(ctx, "init-pipeline", func() (string, error) {
		fmt.Println("Initializing pipeline...")
		time.Sleep(500 * time.Millisecond)
		return "PIPELINE-12345", nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("Pipeline ID: %s\n", pipelineID)

	// Step 2: Process files in parallel
	type FileResult struct {
		Filename string
		Records  int
		Status   string
	}

	results := make([]FileResult, 0, len(dataFiles))

	for i, file := range dataFiles {
		// Capture loop variables
		index := i
		filename := file

		ctx.Go(func() error {
			// Each file gets a unique step ID
			result, err := engine.Step(ctx, fmt.Sprintf("process-file-%d", index), func() (FileResult, error) {
				fmt.Printf("Processing file: %s...\n", filename)
				time.Sleep(1 * time.Second) // Simulate processing
				return FileResult{
					Filename: filename,
					Records:  100 + index*10,
					Status:   "completed",
				}, nil
			})
			if err != nil {
				return err
			}
			results = append(results, result)
			return nil
		})
	}

	// Wait for all files to be processed
	if err := ctx.Wait(); err != nil {
		return fmt.Errorf("file processing failed: %w", err)
	}

	fmt.Println("All files processed successfully")

	// Step 3: Aggregate results
	type AggregateStats struct {
		TotalFiles   int
		TotalRecords int
		ProcessedAt  time.Time
	}

	stats, err := engine.Step(ctx, "aggregate-results", func() (AggregateStats, error) {
		fmt.Println("Aggregating results...")
		time.Sleep(500 * time.Millisecond)

		totalRecords := 0
		for _, result := range results {
			totalRecords += result.Records
		}

		return AggregateStats{
			TotalFiles:   len(dataFiles),
			TotalRecords: totalRecords,
			ProcessedAt:  time.Now(),
		}, nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Pipeline Stats: %d files, %d records\n", stats.TotalFiles, stats.TotalRecords)

	// Step 4: Generate report
	_, err = engine.Step(ctx, "generate-report", func() (string, error) {
		fmt.Println("Generating final report...")
		time.Sleep(500 * time.Millisecond)
		return "report.pdf", nil
	})
	if err != nil {
		return err
	}

	fmt.Println("Pipeline completed successfully!")
	return nil
}

// OrderFulfillment demonstrates a realistic e-commerce workflow
func OrderFulfillment(ctx *engine.Context, orderID string, items []string) error {
	fmt.Printf("Processing order: %s\n", orderID)

	type Order struct {
		ID     string
		Status string
		Total  float64
	}

	// Step 1: Validate order
	order, err := engine.Step(ctx, "validate-order", func() (Order, error) {
		fmt.Println("Validating order...")
		time.Sleep(500 * time.Millisecond)
		return Order{
			ID:     orderID,
			Status: "validated",
			Total:  99.99,
		}, nil
	})
	if err != nil {
		return err
	}

	// Step 2: Reserve inventory (parallel for each item)
	for i, item := range items {
		index := i
		itemName := item

		ctx.Go(func() error {
			_, err := engine.Step(ctx, fmt.Sprintf("reserve-item-%d", index), func() (bool, error) {
				fmt.Printf("Reserving inventory for: %s\n", itemName)
				time.Sleep(1 * time.Second)
				return true, nil
			})
			return err
		})
	}

	if err := ctx.Wait(); err != nil {
		return err
	}

	// Step 3: Process payment
	_, err = engine.Step(ctx, "process-payment", func() (string, error) {
		fmt.Printf("Processing payment of $%.2f...\n", order.Total)
		time.Sleep(1 * time.Second)
		return "PAYMENT-CONFIRMED", nil
	})
	if err != nil {
		return err
	}

	// Step 4: Ship order
	_, err = engine.Step(ctx, "ship-order", func() (string, error) {
		fmt.Println("Shipping order...")
		time.Sleep(500 * time.Millisecond)
		return "TRACKING-123456", nil
	})
	if err != nil {
		return err
	}

	// Step 5: Send confirmation
	_, err = engine.Step(ctx, "send-confirmation", func() (bool, error) {
		fmt.Println("Sending order confirmation email...")
		time.Sleep(500 * time.Millisecond)
		return true, nil
	})

	fmt.Printf("Order %s fulfilled successfully!\n", orderID)
	return err
}
