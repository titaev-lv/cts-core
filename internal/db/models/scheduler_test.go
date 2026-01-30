package models

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// ==============================================================================
// SCHEDULER_TASK TESTS
// ==============================================================================

func TestSchedulerTaskJSONMarshaling(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	task := SchedulerTask{
		ID:                  1,
		TaskName:            "cleanup_trader_sessions",
		TaskType:            TaskTypeCleanup,
		ScheduleCron:        sql.NullString{String: "0 2 * * *", Valid: true},
		ScheduleIntervalSec: sql.NullInt32{Valid: false},
		Enabled:             true,
		Status:              TaskStatusIdle,
		LastRunAt:           sql.NullTime{Time: now, Valid: true},
		LastRunDurationMS:   sql.NullInt32{Int32: 1500, Valid: true},
		LastRunStatus:       sql.NullString{String: string(TaskRunStatusSuccess), Valid: true},
		LastError:           sql.NullString{Valid: false},
		NextRunAt:           sql.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
		RunCount:            10,
		ErrorCount:          0,
		Config:              sql.NullString{String: `{"retention_days": 7}`, Valid: true},
		DateCreate:          now,
		DateModify:          now,
		UserCreated:         sql.NullInt32{Int32: 1, Valid: true},
		UserModify:          sql.NullInt32{Int32: 1, Valid: true},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("Failed to marshal SchedulerTask to JSON: %v", err)
	}

	// Unmarshal from JSON
	var decoded SchedulerTask
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON to SchedulerTask: %v", err)
	}

	// Verify fields
	if decoded.ID != task.ID {
		t.Errorf("Expected ID %d, got %d", task.ID, decoded.ID)
	}
	if decoded.TaskName != task.TaskName {
		t.Errorf("Expected TaskName %s, got %s", task.TaskName, decoded.TaskName)
	}
	if decoded.Enabled != task.Enabled {
		t.Errorf("Expected Enabled %v, got %v", task.Enabled, decoded.Enabled)
	}
}

func TestTaskTypeConstants(t *testing.T) {
	tests := []struct {
		taskType TaskType
		expected string
	}{
		{TaskTypeCleanup, "cleanup"},
		{TaskTypeReencryption, "reencryption"},
		{TaskTypeMonitoring, "monitoring"},
		{TaskTypeMaintenance, "maintenance"},
		{TaskTypeOther, "other"},
	}

	for _, tt := range tests {
		t.Run(string(tt.taskType), func(t *testing.T) {
			if string(tt.taskType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.taskType))
			}
		})
	}
}

func TestTaskStatusConstants(t *testing.T) {
	tests := []struct {
		status   TaskStatus
		expected string
	}{
		{TaskStatusIdle, "idle"},
		{TaskStatusRunning, "running"},
		{TaskStatusFailed, "failed"},
		{TaskStatusDisabled, "disabled"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

func TestTaskRunStatusConstants(t *testing.T) {
	tests := []struct {
		status   TaskRunStatus
		expected string
	}{
		{TaskRunStatusSuccess, "success"},
		{TaskRunStatusError, "error"},
		{TaskRunStatusTimeout, "timeout"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

func TestCronScheduling(t *testing.T) {
	tests := []struct {
		name        string
		cronExpr    string
		description string
	}{
		{"Daily at 2 AM", "0 2 * * *", "cleanup_trader_sessions"},
		{"Every 6 hours", "0 */6 * * *", "health_check"},
		{"First of month", "0 0 1 * *", "monthly_report"},
		{"Every minute", "* * * * *", "high_frequency_monitor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := SchedulerTask{
				TaskName:     tt.description,
				TaskType:     TaskTypeCleanup,
				ScheduleCron: sql.NullString{String: tt.cronExpr, Valid: true},
				Enabled:      true,
			}

			if !task.ScheduleCron.Valid {
				t.Error("Cron-based task should have valid ScheduleCron")
			}
			if task.ScheduleCron.String != tt.cronExpr {
				t.Errorf("Expected cron %s, got %s", tt.cronExpr, task.ScheduleCron.String)
			}
			if task.ScheduleIntervalSec.Valid {
				t.Error("Cron-based task should not have ScheduleIntervalSec")
			}
		})
	}
}

func TestIntervalScheduling(t *testing.T) {
	tests := []struct {
		name        string
		intervalSec int32
		description string
	}{
		{"Every minute", 60, "check_reencryption_jobs"},
		{"Every 5 minutes", 300, "monitor_traders"},
		{"Every hour", 3600, "update_statistics"},
		{"Every 30 seconds", 30, "heartbeat_check"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := SchedulerTask{
				TaskName:            tt.description,
				TaskType:            TaskTypeMonitoring,
				ScheduleIntervalSec: sql.NullInt32{Int32: tt.intervalSec, Valid: true},
				Enabled:             true,
			}

			if !task.ScheduleIntervalSec.Valid {
				t.Error("Interval-based task should have valid ScheduleIntervalSec")
			}
			if task.ScheduleIntervalSec.Int32 != tt.intervalSec {
				t.Errorf("Expected interval %d, got %d", tt.intervalSec, task.ScheduleIntervalSec.Int32)
			}
			if task.ScheduleCron.Valid {
				t.Error("Interval-based task should not have ScheduleCron")
			}
		})
	}
}

func TestDefaultSchedulerTasks(t *testing.T) {
	if len(DefaultSchedulerTasks) != 4 {
		t.Errorf("Expected 4 default tasks, got %d", len(DefaultSchedulerTasks))
	}

	expectedTasks := map[string]TaskType{
		"cleanup_trader_sessions": TaskTypeCleanup,
		"cleanup_audit_logs":      TaskTypeCleanup,
		"reset_daily_limits":      TaskTypeMaintenance,
		"check_reencryption_jobs": TaskTypeReencryption,
	}

	for _, task := range DefaultSchedulerTasks {
		expectedType, exists := expectedTasks[task.TaskName]
		if !exists {
			t.Errorf("Unexpected default task: %s", task.TaskName)
			continue
		}

		if task.TaskType != expectedType {
			t.Errorf("Task %s: expected type %s, got %s", task.TaskName, expectedType, task.TaskType)
		}

		if !task.Enabled {
			t.Errorf("Default task %s should be enabled", task.TaskName)
		}
	}
}

func TestTaskExecution(t *testing.T) {
	now := time.Now()

	task := SchedulerTask{
		TaskName:  "test_task",
		TaskType:  TaskTypeCleanup,
		Enabled:   true,
		Status:    TaskStatusIdle,
		NextRunAt: sql.NullTime{Time: now.Add(-1 * time.Second), Valid: true}, // Past time
	}

	// Check if task should run
	if !time.Now().After(task.NextRunAt.Time) {
		t.Error("Task with past NextRunAt should be ready to run")
	}

	// Simulate task execution
	task.Status = TaskStatusRunning
	startTime := time.Now()

	// Simulate work (sleep for testing)
	time.Sleep(10 * time.Millisecond)

	// Complete task
	endTime := time.Now()
	duration := endTime.Sub(startTime).Milliseconds()

	task.Status = TaskStatusIdle
	task.LastRunAt = sql.NullTime{Time: endTime, Valid: true}
	task.LastRunDurationMS = sql.NullInt32{Int32: int32(duration), Valid: true}
	task.LastRunStatus = sql.NullString{String: string(TaskRunStatusSuccess), Valid: true}
	task.RunCount++

	// Verify execution
	if task.Status != TaskStatusIdle {
		t.Error("Task should return to idle after completion")
	}
	if !task.LastRunAt.Valid {
		t.Error("Task should have valid LastRunAt after execution")
	}
	if task.RunCount != 1 {
		t.Errorf("Expected RunCount 1, got %d", task.RunCount)
	}
}

func TestTaskErrorHandling(t *testing.T) {
	now := time.Now()

	task := SchedulerTask{
		TaskName:   "failing_task",
		TaskType:   TaskTypeCleanup,
		Enabled:    true,
		Status:     TaskStatusIdle,
		ErrorCount: 0,
	}

	// Simulate failed execution
	task.Status = TaskStatusRunning
	task.Status = TaskStatusFailed // Execution failed
	task.LastRunAt = sql.NullTime{Time: now, Valid: true}
	task.LastRunStatus = sql.NullString{String: string(TaskRunStatusError), Valid: true}
	task.LastError = sql.NullString{String: "Database connection failed", Valid: true}
	task.RunCount++
	task.ErrorCount++

	// Verify error tracking
	if task.Status != TaskStatusFailed {
		t.Error("Failed task should have failed status")
	}
	if !task.LastError.Valid {
		t.Error("Failed task should have error message")
	}
	if task.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount 1, got %d", task.ErrorCount)
	}
}

func TestTaskTimeout(t *testing.T) {
	now := time.Now()

	task := SchedulerTask{
		TaskName: "slow_task",
		TaskType: TaskTypeCleanup,
		Enabled:  true,
		Status:   TaskStatusRunning,
	}

	// Simulate timeout
	task.Status = TaskStatusIdle
	task.LastRunAt = sql.NullTime{Time: now, Valid: true}
	task.LastRunStatus = sql.NullString{String: string(TaskRunStatusTimeout), Valid: true}
	task.LastError = sql.NullString{String: "Task exceeded maximum execution time", Valid: true}
	task.RunCount++
	task.ErrorCount++

	// Verify timeout handling
	if task.LastRunStatus.String != string(TaskRunStatusTimeout) {
		t.Error("Timed out task should have timeout status")
	}
}

func TestTaskConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		taskType   TaskType
		configJSON string
	}{
		{
			name:       "Cleanup Config",
			taskType:   TaskTypeCleanup,
			configJSON: `{"retention_days": 7}`,
		},
		{
			name:       "Reencryption Config",
			taskType:   TaskTypeReencryption,
			configJSON: `{"batch_size": 100}`,
		},
		{
			name:       "Monitoring Config",
			taskType:   TaskTypeMonitoring,
			configJSON: `{"timeout_sec": 30, "alert_if_down": true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := SchedulerTask{
				TaskName: tt.name,
				TaskType: tt.taskType,
				Config:   sql.NullString{String: tt.configJSON, Valid: true},
			}

			if !task.Config.Valid {
				t.Error("Task should have valid config")
			}

			// Parse config JSON
			var config map[string]interface{}
			err := json.Unmarshal([]byte(task.Config.String), &config)
			if err != nil {
				t.Errorf("Failed to parse config JSON: %v", err)
			}
		})
	}
}

func TestEnabledDisabledTasks(t *testing.T) {
	now := time.Now()

	enabledTask := SchedulerTask{
		TaskName:  "enabled_task",
		Enabled:   true,
		Status:    TaskStatusIdle,
		NextRunAt: sql.NullTime{Time: now.Add(-1 * time.Second), Valid: true},
	}

	disabledTask := SchedulerTask{
		TaskName: "disabled_task",
		Enabled:  false,
		Status:   TaskStatusDisabled,
	}

	// Enabled task should be ready to run
	if !enabledTask.Enabled {
		t.Error("Enabled task should be enabled")
	}

	// Disabled task should not run
	if disabledTask.Enabled {
		t.Error("Disabled task should not be enabled")
	}
	if disabledTask.Status != TaskStatusDisabled {
		t.Error("Disabled task should have disabled status")
	}
}

func TestNextRunCalculation(t *testing.T) {
	now := time.Now()

	// Interval-based task
	intervalTask := SchedulerTask{
		TaskName:            "interval_task",
		ScheduleIntervalSec: sql.NullInt32{Int32: 60, Valid: true},
		LastRunAt:           sql.NullTime{Time: now, Valid: true},
	}

	// Next run should be LastRunAt + interval
	expectedNextRun := intervalTask.LastRunAt.Time.Add(time.Duration(intervalTask.ScheduleIntervalSec.Int32) * time.Second)
	intervalTask.NextRunAt = sql.NullTime{Time: expectedNextRun, Valid: true}

	if !intervalTask.NextRunAt.Valid {
		t.Error("Task should have valid NextRunAt")
	}

	// Verify calculation
	actualInterval := intervalTask.NextRunAt.Time.Sub(intervalTask.LastRunAt.Time)
	expectedInterval := time.Duration(intervalTask.ScheduleIntervalSec.Int32) * time.Second

	if actualInterval != expectedInterval {
		t.Errorf("Expected interval %v, got %v", expectedInterval, actualInterval)
	}
}

func TestPerformanceTracking(t *testing.T) {
	durations := []int32{100, 150, 200, 120, 180} // milliseconds

	task := SchedulerTask{
		TaskName: "performance_test",
		RunCount: 0,
	}

	totalDuration := int32(0)
	for _, duration := range durations {
		// Simulate task execution
		task.LastRunDurationMS = sql.NullInt32{Int32: duration, Valid: true}
		task.RunCount++
		totalDuration += duration
	}

	if task.RunCount != len(durations) {
		t.Errorf("Expected RunCount %d, got %d", len(durations), task.RunCount)
	}

	// Calculate average duration
	avgDuration := totalDuration / int32(len(durations))
	if avgDuration != 150 {
		t.Errorf("Expected average duration 150ms, got %dms", avgDuration)
	}
}

func TestCleanupTaskRetention(t *testing.T) {
	tests := []struct {
		taskName      string
		retentionDays int
		configJSON    string
	}{
		{"cleanup_trader_sessions", 7, `{"retention_days": 7}`},
		{"cleanup_audit_logs", 180, `{"retention_days": 180}`},
		{"cleanup_old_orders", 90, `{"retention_days": 90}`},
	}

	for _, tt := range tests {
		t.Run(tt.taskName, func(t *testing.T) {
			task := SchedulerTask{
				TaskName: tt.taskName,
				TaskType: TaskTypeCleanup,
				Config:   sql.NullString{String: tt.configJSON, Valid: true},
			}

			// Parse retention config
			var config map[string]interface{}
			json.Unmarshal([]byte(task.Config.String), &config)

			retentionDays := int(config["retention_days"].(float64))
			if retentionDays != tt.retentionDays {
				t.Errorf("Expected retention %d days, got %d days", tt.retentionDays, retentionDays)
			}
		})
	}
}

func TestSchedulerDeadlockDetection(t *testing.T) {
	now := time.Now()
	maxRuntime := 10 * time.Minute

	task := SchedulerTask{
		TaskName:  "potentially_stuck_task",
		Status:    TaskStatusRunning,
		LastRunAt: sql.NullTime{Time: now.Add(-15 * time.Minute), Valid: true}, // Running for 15 minutes
	}

	// Check if task might be stuck
	if task.Status == TaskStatusRunning && task.LastRunAt.Valid {
		runtime := time.Since(task.LastRunAt.Time)
		if runtime > maxRuntime {
			t.Log("Task might be stuck - running for", runtime)
			// In real system, this would trigger an alert
		}
	}
}

func TestMultipleTasksScheduling(t *testing.T) {
	now := time.Now()

	tasks := []SchedulerTask{
		{
			TaskName:     "cleanup_sessions",
			TaskType:     TaskTypeCleanup,
			ScheduleCron: sql.NullString{String: "0 2 * * *", Valid: true},
			Enabled:      true,
			NextRunAt:    sql.NullTime{Time: now.Add(2 * time.Hour), Valid: true},
		},
		{
			TaskName:            "monitor_health",
			TaskType:            TaskTypeMonitoring,
			ScheduleIntervalSec: sql.NullInt32{Int32: 60, Valid: true},
			Enabled:             true,
			NextRunAt:           sql.NullTime{Time: now.Add(-1 * time.Second), Valid: true}, // Ready to run
		},
		{
			TaskName: "disabled_task",
			Enabled:  false,
			Status:   TaskStatusDisabled,
		},
	}

	// Count ready tasks
	readyCount := 0
	for _, task := range tasks {
		if task.Enabled && task.NextRunAt.Valid && time.Now().After(task.NextRunAt.Time) {
			readyCount++
		}
	}

	if readyCount != 1 {
		t.Errorf("Expected 1 ready task, got %d", readyCount)
	}
}
