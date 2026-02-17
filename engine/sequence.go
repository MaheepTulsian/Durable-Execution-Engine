package engine

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// generateStepKey creates a unique key combining step ID and sequence number
// Format: "stepID:sequenceNum" (e.g., "create-user:1", "send-email:2")
func generateStepKey(stepID string, sequenceNum int64) string {
	return fmt.Sprintf("%s:%d", stepID, sequenceNum)
}

// getCallerLocation returns the file and line number of the caller
// This is used for automatic step ID generation
func getCallerLocation(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown:0"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}
