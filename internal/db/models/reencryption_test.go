package models

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// ==============================================================================
// REENCRYPTION_JOBS TESTS
// ==============================================================================

func TestReencryptionJobJSONMarshaling(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	job := ReencryptionJob{
		ID:               1,
		JobType:          ReencryptionJobTypeUser2FA,
		OldKeyVersion:    1,
		NewKeyVersion:    2,
		Context:          "2fa",
		Status:           ReencryptionJobStatusInProgress,
		TotalRecords:     5000,
		ProcessedRecords: 2500,
		FailedRecords:    5,
		BatchSize:        100,
		StartedAt:        sql.NullTime{Time: now, Valid: true},
		CompletedAt:      sql.NullTime{Valid: false},
		LastProcessedAt:  sql.NullTime{Time: now, Valid: true},
		ErrorMessage:     sql.NullString{Valid: false},
		DateCreate:       now,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("Failed to marshal ReencryptionJob to JSON: %v", err)
	}

	// Unmarshal from JSON
	var decoded ReencryptionJob
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON to ReencryptionJob: %v", err)
	}

	// Verify fields
	if decoded.ID != job.ID {
		t.Errorf("Expected ID %d, got %d", job.ID, decoded.ID)
	}
	if decoded.JobType != job.JobType {
		t.Errorf("Expected JobType %s, got %s", job.JobType, decoded.JobType)
	}
	if decoded.TotalRecords != job.TotalRecords {
		t.Errorf("Expected TotalRecords %d, got %d", job.TotalRecords, decoded.TotalRecords)
	}
	if decoded.ProcessedRecords != job.ProcessedRecords {
		t.Errorf("Expected ProcessedRecords %d, got %d", job.ProcessedRecords, decoded.ProcessedRecords)
	}
}

func TestReencryptionJobTypeConstants(t *testing.T) {
	tests := []struct {
		jobType  ReencryptionJobType
		expected string
	}{
		{ReencryptionJobTypeUser2FA, "user_2fa"},
		{ReencryptionJobTypeExchangeAccounts, "exchange_accounts"},
		{ReencryptionJobTypeOther, "other"},
	}

	for _, tt := range tests {
		t.Run(string(tt.jobType), func(t *testing.T) {
			if string(tt.jobType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.jobType))
			}
		})
	}
}

func TestReencryptionJobStatusConstants(t *testing.T) {
	tests := []struct {
		status   ReencryptionJobStatus
		expected string
	}{
		{ReencryptionJobStatusPending, "pending"},
		{ReencryptionJobStatusInProgress, "in_progress"},
		{ReencryptionJobStatusCompleted, "completed"},
		{ReencryptionJobStatusFailed, "failed"},
		{ReencryptionJobStatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

func TestReencryptionJobLifecycle(t *testing.T) {
	now := time.Now()

	// Create pending job
	job := ReencryptionJob{
		ID:            1,
		JobType:       ReencryptionJobTypeUser2FA,
		OldKeyVersion: 1,
		NewKeyVersion: 2,
		Context:       "2fa",
		Status:        ReencryptionJobStatusPending,
		TotalRecords:  5000,
		BatchSize:     100,
		DateCreate:    now,
	}

	if job.Status != ReencryptionJobStatusPending {
		t.Errorf("Expected initial status %s, got %s", ReencryptionJobStatusPending, job.Status)
	}
	if job.StartedAt.Valid {
		t.Error("Pending job should not have StartedAt timestamp")
	}

	// Start processing
	job.Status = ReencryptionJobStatusInProgress
	job.StartedAt = sql.NullTime{Time: now, Valid: true}

	if job.Status != ReencryptionJobStatusInProgress {
		t.Errorf("Expected status %s, got %s", ReencryptionJobStatusInProgress, job.Status)
	}
	if !job.StartedAt.Valid {
		t.Error("In-progress job should have valid StartedAt timestamp")
	}

	// Complete job
	job.Status = ReencryptionJobStatusCompleted
	job.ProcessedRecords = job.TotalRecords
	job.CompletedAt = sql.NullTime{Time: now.Add(1 * time.Hour), Valid: true}

	if job.Status != ReencryptionJobStatusCompleted {
		t.Errorf("Expected status %s, got %s", ReencryptionJobStatusCompleted, job.Status)
	}
	if !job.CompletedAt.Valid {
		t.Error("Completed job should have valid CompletedAt timestamp")
	}
}

func TestReencryptionProgressTracking(t *testing.T) {
	tests := []struct {
		name             string
		totalRecords     int
		processedRecords int
		failedRecords    int
		expectedPercent  int
	}{
		{"0% Complete", 5000, 0, 0, 0},
		{"50% Complete", 5000, 2500, 0, 50},
		{"99% Complete", 5000, 4950, 0, 99},
		{"100% Complete", 5000, 5000, 0, 100},
		{"With Failures", 5000, 4900, 100, 98},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := ReencryptionJob{
				TotalRecords:     tt.totalRecords,
				ProcessedRecords: tt.processedRecords,
				FailedRecords:    tt.failedRecords,
			}

			// Calculate completion percentage
			percent := 0
			if job.TotalRecords > 0 {
				percent = (job.ProcessedRecords * 100) / job.TotalRecords
			}

			if percent != tt.expectedPercent {
				t.Errorf("Expected %d%% complete, got %d%%", tt.expectedPercent, percent)
			}
		})
	}
}

func TestBatchSizeConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
	}{
		{"Small Batch", 10},
		{"Default Batch", 100},
		{"Large Batch", 1000},
		{"Extra Large Batch", 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := ReencryptionJob{
				BatchSize: tt.batchSize,
			}

			if job.BatchSize != tt.batchSize {
				t.Errorf("Expected batch size %d, got %d", tt.batchSize, job.BatchSize)
			}
		})
	}
}

func TestKeyVersionMigration(t *testing.T) {
	tests := []struct {
		name          string
		oldVersion    int
		newVersion    int
		context       string
		expectedKeyID string
	}{
		{"2FA v1 to v2", 1, 2, "2fa", "kek-2fa-v2"},
		{"Exchange v1 to v2", 1, 2, "exchange-key", "kek-exchange-key-v2"},
		{"2FA v2 to v3", 2, 3, "2fa", "kek-2fa-v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := ReencryptionJob{
				OldKeyVersion: tt.oldVersion,
				NewKeyVersion: tt.newVersion,
				Context:       tt.context,
			}

			if job.OldKeyVersion != tt.oldVersion {
				t.Errorf("Expected old version %d, got %d", tt.oldVersion, job.OldKeyVersion)
			}
			if job.NewKeyVersion != tt.newVersion {
				t.Errorf("Expected new version %d, got %d", tt.newVersion, job.NewKeyVersion)
			}
			if job.Context != tt.context {
				t.Errorf("Expected context %s, got %s", tt.context, job.Context)
			}
		})
	}
}

func TestJobFailureHandling(t *testing.T) {
	now := time.Now()

	job := ReencryptionJob{
		ID:               1,
		JobType:          ReencryptionJobTypeExchangeAccounts,
		Status:           ReencryptionJobStatusFailed,
		TotalRecords:     1000,
		ProcessedRecords: 500,
		FailedRecords:    500,
		StartedAt:        sql.NullTime{Time: now, Valid: true},
		ErrorMessage:     sql.NullString{String: "HSM connection timeout", Valid: true},
	}

	if job.Status != ReencryptionJobStatusFailed {
		t.Error("Job should have failed status")
	}
	if !job.ErrorMessage.Valid {
		t.Error("Failed job should have error message")
	}
	if job.ErrorMessage.String != "HSM connection timeout" {
		t.Errorf("Expected error message 'HSM connection timeout', got '%s'", job.ErrorMessage.String)
	}
}

// ==============================================================================
// REENCRYPTION_PROGRESS TESTS
// ==============================================================================

func TestReencryptionProgressJSONMarshaling(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	progress := ReencryptionProgress{
		ID:           1,
		JobID:        1,
		TableName:    "USER_2FA",
		RecordID:     "123",
		Status:       ReencryptionProgressStatusCompleted,
		AttemptCount: 1,
		ErrorMessage: sql.NullString{Valid: false},
		ProcessedAt:  sql.NullTime{Time: now, Valid: true},
		DateCreate:   now,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(progress)
	if err != nil {
		t.Fatalf("Failed to marshal ReencryptionProgress to JSON: %v", err)
	}

	// Unmarshal from JSON
	var decoded ReencryptionProgress
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON to ReencryptionProgress: %v", err)
	}

	// Verify fields
	if decoded.ID != progress.ID {
		t.Errorf("Expected ID %d, got %d", progress.ID, decoded.ID)
	}
	if decoded.TableName != progress.TableName {
		t.Errorf("Expected TableName %s, got %s", progress.TableName, decoded.TableName)
	}
	if decoded.Status != progress.Status {
		t.Errorf("Expected Status %s, got %s", progress.Status, decoded.Status)
	}
}

func TestReencryptionProgressStatusConstants(t *testing.T) {
	tests := []struct {
		status   ReencryptionProgressStatus
		expected string
	}{
		{ReencryptionProgressStatusPending, "pending"},
		{ReencryptionProgressStatusCompleted, "completed"},
		{ReencryptionProgressStatusFailed, "failed"},
		{ReencryptionProgressStatusSkipped, "skipped"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

func TestProgressRecordTracking(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		recordID  string
	}{
		{"USER_2FA Record", "USER_2FA", "123"},
		{"EXCHANGE_ACCOUNTS Record", "EXCHANGE_ACCOUNTS", "456789"},
		{"Large ID", "USER_2FA", "999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress := ReencryptionProgress{
				TableName: tt.tableName,
				RecordID:  tt.recordID,
				Status:    ReencryptionProgressStatusPending,
			}

			if progress.TableName != tt.tableName {
				t.Errorf("Expected TableName %s, got %s", tt.tableName, progress.TableName)
			}
			if progress.RecordID != tt.recordID {
				t.Errorf("Expected RecordID %s, got %s", tt.recordID, progress.RecordID)
			}
		})
	}
}

func TestRetryMechanism(t *testing.T) {
	maxRetries := 3

	progress := ReencryptionProgress{
		ID:           1,
		JobID:        1,
		TableName:    "USER_2FA",
		RecordID:     "123",
		Status:       ReencryptionProgressStatusPending,
		AttemptCount: 0,
	}

	// Simulate retry attempts
	for i := 1; i <= maxRetries; i++ {
		progress.AttemptCount++
		progress.Status = ReencryptionProgressStatusFailed
		progress.ErrorMessage = sql.NullString{String: "HSM decrypt failed", Valid: true}

		if progress.AttemptCount != i {
			t.Errorf("Expected attempt count %d, got %d", i, progress.AttemptCount)
		}
	}

	// After max retries, should give up
	if progress.AttemptCount >= maxRetries {
		if progress.Status != ReencryptionProgressStatusFailed {
			t.Error("Should remain failed after max retries")
		}
	}
}

func TestSuccessfulReencryption(t *testing.T) {
	now := time.Now()

	progress := ReencryptionProgress{
		ID:           1,
		JobID:        1,
		TableName:    "USER_2FA",
		RecordID:     "123",
		Status:       ReencryptionProgressStatusPending,
		AttemptCount: 0,
		DateCreate:   now,
	}

	// First attempt succeeds
	progress.Status = ReencryptionProgressStatusCompleted
	progress.AttemptCount = 1
	progress.ProcessedAt = sql.NullTime{Time: now.Add(1 * time.Second), Valid: true}

	if progress.Status != ReencryptionProgressStatusCompleted {
		t.Error("Successful reencryption should be completed")
	}
	if !progress.ProcessedAt.Valid {
		t.Error("Completed progress should have valid ProcessedAt timestamp")
	}
	if progress.AttemptCount != 1 {
		t.Errorf("Expected 1 attempt, got %d", progress.AttemptCount)
	}
}

func TestSkippedRecords(t *testing.T) {
	// Record already processed or not applicable
	progress := ReencryptionProgress{
		ID:        1,
		JobID:     1,
		TableName: "USER_2FA",
		RecordID:  "123",
		Status:    ReencryptionProgressStatusSkipped,
	}

	if progress.Status != ReencryptionProgressStatusSkipped {
		t.Error("Record should be marked as skipped")
	}
	if progress.ProcessedAt.Valid {
		t.Error("Skipped record should not have ProcessedAt timestamp")
	}
}

func TestBatchProcessingSimulation(t *testing.T) {
	jobID := 1
	batchSize := 100

	// Create batch of progress records
	batch := make([]ReencryptionProgress, batchSize)
	for i := 0; i < batchSize; i++ {
		batch[i] = ReencryptionProgress{
			ID:        int64(i + 1),
			JobID:     jobID,
			TableName: "USER_2FA",
			RecordID:  string(rune(i + 1)),
			Status:    ReencryptionProgressStatusPending,
		}
	}

	if len(batch) != batchSize {
		t.Errorf("Expected batch size %d, got %d", batchSize, len(batch))
	}

	// Simulate processing
	completed := 0
	failed := 0

	for i := range batch {
		// Simulate 95% success rate
		if i%20 == 0 {
			batch[i].Status = ReencryptionProgressStatusFailed
			batch[i].ErrorMessage = sql.NullString{String: "Decrypt error", Valid: true}
			failed++
		} else {
			batch[i].Status = ReencryptionProgressStatusCompleted
			batch[i].ProcessedAt = sql.NullTime{Time: time.Now(), Valid: true}
			completed++
		}
	}

	if completed+failed != batchSize {
		t.Error("All records should be either completed or failed")
	}
}

func TestErrorMessageDetails(t *testing.T) {
	tests := []struct {
		name         string
		errorMessage string
	}{
		{"HSM Decrypt Error", "HSM decrypt failed: invalid ciphertext"},
		{"Database Error", "Database update failed: connection timeout"},
		{"Invalid Key Version", "Key version mismatch: expected v2, got v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress := ReencryptionProgress{
				Status:       ReencryptionProgressStatusFailed,
				ErrorMessage: sql.NullString{String: tt.errorMessage, Valid: true},
			}

			if !progress.ErrorMessage.Valid {
				t.Error("Failed progress should have valid error message")
			}
			if progress.ErrorMessage.String != tt.errorMessage {
				t.Errorf("Expected error message '%s', got '%s'", tt.errorMessage, progress.ErrorMessage.String)
			}
		})
	}
}

func TestJobAndProgressRelationship(t *testing.T) {
	jobID := 1
	recordCount := 5

	// Create job
	job := ReencryptionJob{
		ID:           jobID,
		TotalRecords: recordCount,
	}

	// Create progress records for the job
	progressRecords := make([]ReencryptionProgress, recordCount)
	for i := 0; i < recordCount; i++ {
		progressRecords[i] = ReencryptionProgress{
			ID:        int64(i + 1),
			JobID:     jobID,
			TableName: "USER_2FA",
			RecordID:  string(rune(i + 100)),
			Status:    ReencryptionProgressStatusPending,
		}
	}

	// Verify all progress records belong to the job
	for _, progress := range progressRecords {
		if progress.JobID != job.ID {
			t.Errorf("Progress record should belong to job %d, got %d", job.ID, progress.JobID)
		}
	}

	if len(progressRecords) != job.TotalRecords {
		t.Errorf("Expected %d progress records, got %d", job.TotalRecords, len(progressRecords))
	}
}
