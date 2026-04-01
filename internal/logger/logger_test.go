package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestInit(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		dir           string
		maxFileSizeMB int
		maxBackups    int
		maxAgeDays    int
		compress      bool
		expectedLevel slog.Level
		expectError   bool
	}{
		{
			name:          "debug level",
			level:         "debug",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			maxBackups:    3,
			maxAgeDays:    7,
			compress:      true,
			expectedLevel: slog.LevelDebug,
			expectError:   false,
		},
		{
			name:          "info level",
			level:         "info",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			maxBackups:    3,
			maxAgeDays:    7,
			compress:      true,
			expectedLevel: slog.LevelInfo,
			expectError:   false,
		},
		{
			name:          "warn level",
			level:         "warn",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			maxBackups:    3,
			maxAgeDays:    7,
			compress:      true,
			expectedLevel: slog.LevelWarn,
			expectError:   false,
		},
		{
			name:          "error level",
			level:         "error",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			maxBackups:    3,
			maxAgeDays:    7,
			compress:      true,
			expectedLevel: slog.LevelError,
			expectError:   false,
		},
		{
			name:          "unknown level defaults to info",
			level:         "unknown",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			maxBackups:    3,
			maxAgeDays:    7,
			compress:      true,
			expectedLevel: slog.LevelInfo,
			expectError:   false,
		},
		{
			name:          "case insensitive",
			level:         "DEBUG",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			maxBackups:    3,
			maxAgeDays:    7,
			compress:      true,
			expectedLevel: slog.LevelDebug,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InitWithOptions(Options{
				Level:         tt.level,
				Dir:           tt.dir,
				MaxFileSizeMB: tt.maxFileSizeMB,
				MaxBackups:    tt.maxBackups,
				MaxAgeDays:    tt.maxAgeDays,
				Compress:      tt.compress,
			})
			if (err != nil) != tt.expectError {
				t.Errorf("Init() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if !tt.expectError {
				// Check that Log is initialized
				if Log == nil {
					t.Error("Log is nil after Init()")
				}

				// Check log level
				if GetLevel() != tt.expectedLevel {
					t.Errorf("GetLevel() = %v, want %v", GetLevel(), tt.expectedLevel)
				}

				// Check log directory
				if GetLogDir() != tt.dir {
					t.Errorf("GetLogDir() = %v, want %v", GetLogDir(), tt.dir)
				}

				switch tt.expectedLevel {
				case slog.LevelDebug:
					Debug("init test message")
				case slog.LevelWarn:
					Warn("init test message")
				case slog.LevelError:
					Error("init test message")
				default:
					Info("init test message")
				}
				if err := Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}

				// Check error.log file exists after first write
				errorLogPath := filepath.Join(tt.dir, "error.log")
				if _, err := os.Stat(errorLogPath); os.IsNotExist(err) {
					t.Errorf("error.log does not exist at %s", errorLogPath)
				}
			}
		})
	}
}

func TestLogLevels(t *testing.T) {
	logDir := t.TempDir()
	if err := InitWithOptions(Options{
		Level:              "debug",
		Dir:                logDir,
		MaxFileSizeMB:      10,
		MaxBackups:         3,
		MaxAgeDays:         7,
		Compress:           true,
		AccessToStdout:     false,
		OutRequestToStdout: false,
		WSInToStdout:       false,
		WSOutToStdout:      false,
		AuditToStdout:      false,
	}); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	// Test Debug
	Debug("debug message", "key", "value")

	// Test Info
	Info("info message", "key", "value")

	// Test Warn
	Warn("warn message", "key", "value")

	// Test Error
	Error("error message", "key", "value")

	if err := Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Read log file
	errorLogPath := filepath.Join(logDir, "error.log")
	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log: %v", err)
	}

	logContent := string(content)

	// Check all messages are logged
	if !strings.Contains(logContent, "debug message") {
		t.Error("debug message not found in log")
	}
	if !strings.Contains(logContent, "info message") {
		t.Error("info message not found in log")
	}
	if !strings.Contains(logContent, "warn message") {
		t.Error("warn message not found in log")
	}
	if !strings.Contains(logContent, "error message") {
		t.Error("error message not found in log")
	}

	// Check JSON fields are present
	if !strings.Contains(logContent, "\"level\":\"DEBUG\"") {
		t.Error("DEBUG level not found in JSON log")
	}
	if !strings.Contains(logContent, "\"level\":\"INFO\"") {
		t.Error("INFO level not found in JSON log")
	}
	if !strings.Contains(logContent, "\"level\":\"WARN\"") {
		t.Error("WARN level not found in JSON log")
	}
	if !strings.Contains(logContent, "\"level\":\"ERROR\"") {
		t.Error("ERROR level not found in JSON log")
	}

	// Check attributes are present
	if !strings.Contains(logContent, "\"key\":\"value\"") {
		t.Error("key=value attributes not found in JSON log")
	}
}

func TestLogLevelFiltering(t *testing.T) {
	logDir := t.TempDir()
	// Set level to INFO (should filter out DEBUG)
	if err := InitWithOptions(Options{
		Level:         "info",
		Dir:           logDir,
		MaxFileSizeMB: 10,
		MaxBackups:    3,
		MaxAgeDays:    7,
		Compress:      true,
	}); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	Debug("debug message should not appear")
	Info("info message should appear")

	if err := Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Read log file
	errorLogPath := filepath.Join(logDir, "error.log")
	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log: %v", err)
	}

	logContent := string(content)

	// Debug message should NOT be in log
	if strings.Contains(logContent, "debug message should not appear") {
		t.Error("DEBUG message logged when level is INFO")
	}

	// Info message SHOULD be in log
	if !strings.Contains(logContent, "info message should appear") {
		t.Error("INFO message not logged when level is INFO")
	}
}

func TestModularLogger(t *testing.T) {
	logDir := t.TempDir()
	if err := InitWithOptions(Options{
		Level:         "info",
		Dir:           logDir,
		MaxFileSizeMB: 10,
		MaxBackups:    3,
		MaxAgeDays:    7,
		Compress:      true,
	}); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	// Get modular loggers
	mainLogger := Get("main")
	dbLogger := Get("database")
	apiLogger := Get("api")
	accessLogger := GetAccess("api")
	wsLogger := GetWSAccess("ws")

	// Log messages from different modules
	mainLogger.Info("message from main")
	dbLogger.Info("message from database")
	apiLogger.Info("message from api")
	accessLogger.Info("message from access")
	wsLogger.Info("message from ws access")

	if err := Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Read log file
	errorLogPath := filepath.Join(logDir, "error.log")
	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log: %v", err)
	}

	logContent := string(content)

	// Check module names appear in logs
	if !strings.Contains(logContent, "\"module\":\"main\"") {
		t.Error("main module not found in JSON log")
	}
	if !strings.Contains(logContent, "\"module\":\"database\"") {
		t.Error("database module not found in JSON log")
	}
	if !strings.Contains(logContent, "\"module\":\"api\"") {
		t.Error("api module not found in JSON log")
	}

	// Check messages
	if !strings.Contains(logContent, "message from main") {
		t.Error("main module message not found")
	}
	if !strings.Contains(logContent, "message from database") {
		t.Error("database module message not found")
	}
	if !strings.Contains(logContent, "message from api") {
		t.Error("api module message not found")
	}
}

func TestRotation(t *testing.T) {
	logDir := t.TempDir()

	if err := Init("info", logDir, 1, 3, 7, false); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	Info("rotation baseline")

	preRotateInfo, err := os.Stat(filepath.Join(logDir, "error.log"))
	if err != nil {
		t.Fatalf("error.log does not exist before rotation: %v", err)
	}
	preRotateSize := preRotateInfo.Size()

	rotator, ok := logFiles["error"].(*lumberjack.Logger)
	if !ok {
		t.Fatalf("expected lumberjack logger, got %T", logFiles["error"])
	}
	if err := rotator.Rotate(); err != nil {
		t.Fatalf("Rotate() failed: %v", err)
	}

	Info("rotation after")

	if err := Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Check that backup file was created (lumberjack uses numeric suffixes)
	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	hasBackup := false
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "error.log.") {
			hasBackup = true
			t.Logf("Found backup file: %s", file.Name())
			break
		}
	}

	// Check that current error.log exists
	errorLogPath := filepath.Join(logDir, "error.log")
	info, err := os.Stat(errorLogPath)
	if err != nil {
		t.Fatalf("error.log does not exist after rotation: %v", err)
	}

	// Current file should exist (size may vary)
	t.Logf("Current error.log size: %d bytes", info.Size())

	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log after rotation: %v", err)
	}
	if !strings.Contains(string(content), "rotation after") {
		t.Error("rotation after message not found in log")
	}

	if !hasBackup && info.Size() >= preRotateSize {
		t.Errorf("No backup file created after rotation and size did not decrease (before=%d after=%d)", preRotateSize, info.Size())
	}
}

func TestRotateOnStartup(t *testing.T) {
	logDir := t.TempDir()
	errorLogPath := filepath.Join(logDir, "error.log")

	if err := os.WriteFile(errorLogPath, []byte("previous-run-entry\n"), 0640); err != nil {
		t.Fatalf("failed to seed existing error.log: %v", err)
	}

	if err := InitWithOptions(Options{
		Level:              "info",
		Dir:                logDir,
		MaxFileSizeMB:      10,
		MaxBackups:         3,
		MaxAgeDays:         7,
		Compress:           false,
		ErrorPath:          errorLogPath,
		AccessToStdout:     false,
		OutRequestToStdout: false,
		WSInToStdout:       false,
		WSOutToStdout:      false,
		AuditToStdout:      false,
	}); err != nil {
		t.Fatalf("InitWithOptions() failed: %v", err)
	}

	Info("current-run-entry")

	if err := Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("failed to read current error.log: %v", err)
	}
	logContent := string(content)
	if strings.Contains(logContent, "previous-run-entry") {
		t.Fatalf("previous run entry leaked into current error.log")
	}
	if !strings.Contains(logContent, "current-run-entry") {
		t.Fatalf("current run entry missing from error.log")
	}

}

func TestClose(t *testing.T) {
	logDir := t.TempDir()
	if err := Init("info", logDir, 10, 3, 7, true); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	Info("test message before close")

	// Close logger
	if err := Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Check that log file still exists and contains the message
	errorLogPath := filepath.Join(logDir, "error.log")
	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log after close: %v", err)
	}

	if !strings.Contains(string(content), "test message before close") {
		t.Error("Message not found in log after close")
	}
}

func TestGetBeforeInit(t *testing.T) {
	// Reset Log to nil to simulate uninitialized state
	oldLog := Log
	Log = nil
	defer func() { Log = oldLog }()

	// Get() should return a fallback logger, not panic
	logger := Get("test")
	if logger == nil {
		t.Error("Get() returned nil when Log is not initialized")
	}

	// Should be able to log without panic
	logger.Info("fallback message")
}

func TestLogFormat(t *testing.T) {
	logDir := t.TempDir()
	if err := Init("info", logDir, 10, 3, 7, true); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	Get("main").Info("test message", "key1", "value1", "key2", "value2")

	if err := Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	errorLogPath := filepath.Join(logDir, "error.log")
	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log: %v", err)
	}

	logContent := string(content)

	if !strings.Contains(logContent, "\"level\":\"INFO\"") {
		t.Error("INFO level not found in JSON log")
	}
	if !strings.Contains(logContent, "\"module\":\"main\"") {
		t.Error("main module not found in JSON log")
	}
	if !strings.Contains(logContent, "\"msg\":\"test message\"") {
		t.Error("Message not found in JSON log")
	}
	if !strings.Contains(logContent, "\"key1\":\"value1\"") {
		t.Error("key1=value1 not found in JSON log")
	}
	if !strings.Contains(logContent, "\"key2\":\"value2\"") {
		t.Error("key2=value2 not found in JSON log")
	}
	if !strings.Contains(logContent, "\"time\":\"") || !strings.Contains(logContent, "Z\"") {
		t.Error("Timestamp format not found in JSON log")
	}
}
