package engine

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

// NewStorage creates a new storage instance with SQLite database
func NewStorage(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure SQLite for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// SQLite single-writer limitation
	db.SetMaxOpenConns(1)

	s := &Storage{db: db}

	// Initialize schema
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return s, nil
}

// initSchema creates the database tables if they don't exist
func (s *Storage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS workflows (
		workflow_id TEXT PRIMARY KEY,
		status TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS steps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workflow_id TEXT NOT NULL,
		step_id TEXT NOT NULL,
		sequence_num INTEGER NOT NULL,
		step_key TEXT UNIQUE NOT NULL,
		status TEXT NOT NULL,
		output BLOB,
		error TEXT,
		started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP,
		FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
	);

	CREATE INDEX IF NOT EXISTS idx_workflow_steps ON steps(workflow_id, sequence_num);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_step_key ON steps(step_key);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// CreateWorkflow creates a new workflow record
func (s *Storage) CreateWorkflow(workflowID string) error {
	return s.retryOnBusy(func() error {
		_, err := s.db.Exec(
			"INSERT OR IGNORE INTO workflows (workflow_id, status) VALUES (?, ?)",
			workflowID, "running",
		)
		return err
	})
}

// UpdateWorkflowStatus updates the status of a workflow
func (s *Storage) UpdateWorkflowStatus(workflowID, status string) error {
	return s.retryOnBusy(func() error {
		_, err := s.db.Exec(
			"UPDATE workflows SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE workflow_id = ?",
			status, workflowID,
		)
		return err
	})
}

// GetStep retrieves a completed step's result
func (s *Storage) GetStep(workflowID, stepKey string) ([]byte, bool, error) {
	var output []byte
	var status string

	err := s.db.QueryRow(
		"SELECT output, status FROM steps WHERE workflow_id = ? AND step_key = ?",
		workflowID, stepKey,
	).Scan(&output, &status)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get step: %w", err)
	}

	// Only return completed steps
	if status != "completed" {
		return nil, false, nil
	}

	return output, true, nil
}

// MarkStepInProgress marks a step as started (for zombie detection)
func (s *Storage) MarkStepInProgress(workflowID, stepKey, stepID string, sequenceNum int64) error {
	return s.retryOnBusy(func() error {
		_, err := s.db.Exec(
			`INSERT INTO steps (workflow_id, step_key, step_id, sequence_num, status)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(step_key) DO UPDATE SET status = 'in_progress'`,
			workflowID, stepKey, stepID, sequenceNum, "in_progress",
		)
		return err
	})
}

// SaveStep persists a step's result
func (s *Storage) SaveStep(workflowID, stepKey string, output []byte) error {
	return s.retryOnBusy(func() error {
		_, err := s.db.Exec(
			`UPDATE steps
			 SET status = 'completed', output = ?, completed_at = CURRENT_TIMESTAMP
			 WHERE workflow_id = ? AND step_key = ?`,
			output, workflowID, stepKey,
		)
		return err
	})
}

// SaveStepError saves an error for a failed step
func (s *Storage) SaveStepError(workflowID, stepKey string, errMsg string) error {
	return s.retryOnBusy(func() error {
		_, err := s.db.Exec(
			`UPDATE steps
			 SET status = 'failed', error = ?, completed_at = CURRENT_TIMESTAMP
			 WHERE workflow_id = ? AND step_key = ?`,
			errMsg, workflowID, stepKey,
		)
		return err
	})
}

// GetMaxSequenceNum returns the maximum sequence number for a workflow
func (s *Storage) GetMaxSequenceNum(workflowID string) (int64, error) {
	var maxSeq sql.NullInt64
	err := s.db.QueryRow(
		"SELECT MAX(sequence_num) FROM steps WHERE workflow_id = ?",
		workflowID,
	).Scan(&maxSeq)

	if err != nil {
		return 0, fmt.Errorf("failed to get max sequence: %w", err)
	}

	if !maxSeq.Valid {
		return 0, nil
	}

	return maxSeq.Int64, nil
}

// LoadCompletedSteps loads all completed steps for a workflow
func (s *Storage) LoadCompletedSteps(workflowID string) (map[string][]byte, error) {
	rows, err := s.db.Query(
		"SELECT step_key, output FROM steps WHERE workflow_id = ? AND status = 'completed'",
		workflowID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load steps: %w", err)
	}
	defer rows.Close()

	steps := make(map[string][]byte)
	for rows.Next() {
		var stepKey string
		var output []byte
		if err := rows.Scan(&stepKey, &output); err != nil {
			return nil, fmt.Errorf("failed to scan step: %w", err)
		}
		steps[stepKey] = output
	}

	return steps, rows.Err()
}

// LoadStepIDMapping loads the mapping of step IDs to sequence numbers
func (s *Storage) LoadStepIDMapping(workflowID string) (map[string]int64, error) {
	rows, err := s.db.Query(
		"SELECT step_id, sequence_num FROM steps WHERE workflow_id = ?",
		workflowID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load step ID mapping: %w", err)
	}
	defer rows.Close()

	mapping := make(map[string]int64)
	for rows.Next() {
		var stepID string
		var seqNum int64
		if err := rows.Scan(&stepID, &seqNum); err != nil {
			return nil, fmt.Errorf("failed to scan step mapping: %w", err)
		}
		mapping[stepID] = seqNum
	}

	return mapping, rows.Err()
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// retryOnBusy retries a database operation if SQLite is busy
func (s *Storage) retryOnBusy(fn func() error) error {
	maxRetries := 5
	var err error

	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}

		// Check if it's a busy error
		if !isSQLiteBusy(err) {
			return err
		}

		// Exponential backoff
		time.Sleep(time.Millisecond * time.Duration(10*(i+1)))
	}

	return fmt.Errorf("max retries exceeded: %w", err)
}

// isSQLiteBusy checks if an error is a SQLite busy error
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "database is locked")
}

// GetWorkflowStatus returns the current status of a workflow
func (s *Storage) GetWorkflowStatus(workflowID string) (string, error) {
	var status string
	err := s.db.QueryRow(
		"SELECT status FROM workflows WHERE workflow_id = ?",
		workflowID,
	).Scan(&status)

	if err == sql.ErrNoRows {
		return "", errors.New("workflow not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to get workflow status: %w", err)
	}

	return status, nil
}
