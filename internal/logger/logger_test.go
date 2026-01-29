package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		dir           string
		maxFileSizeMB int
		expectedLevel slog.Level
		expectError   bool
	}{
		{
			name:          "debug level",
			level:         "debug",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			expectedLevel: slog.LevelDebug,
			expectError:   false,
		},
		{
			name:          "info level",
			level:         "info",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			expectedLevel: slog.LevelInfo,
			expectError:   false,
		},
		{
			name:          "warn level",
			level:         "warn",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			expectedLevel: slog.LevelWarn,
			expectError:   false,
		},
		{
			name:          "error level",
			level:         "error",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			expectedLevel: slog.LevelError,
			expectError:   false,
		},
		{
			name:          "unknown level defaults to info",
			level:         "unknown",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			expectedLevel: slog.LevelInfo,
			expectError:   false,
		},
		{
			name:          "case insensitive",
			level:         "DEBUG",
			dir:           t.TempDir(),
			maxFileSizeMB: 10,
			expectedLevel: slog.LevelDebug,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Init(tt.level, tt.dir, tt.maxFileSizeMB)
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

				// Check error.log file exists
				errorLogPath := filepath.Join(tt.dir, "error.log")
				if _, err := os.Stat(errorLogPath); os.IsNotExist(err) {
					t.Errorf("error.log does not exist at %s", errorLogPath)
				}

				// Cleanup
				Close()
			}
		})
	}
}

func TestLogLevels(t *testing.T) {
	logDir := t.TempDir()
	if err := Init("debug", logDir, 10); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	defer Close()

	// Test Debug
	Debug("debug message", "key", "value")

	// Test Info
	Info("info message", "key", "value")

	// Test Warn
	Warn("warn message", "key", "value")

	// Test Error
	Error("error message", "key", "value")

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

	// Check log format contains [DEBUG], [INFO], [WARN], [ERROR]
	if !strings.Contains(logContent, "[DEBUG]") {
		t.Error("[DEBUG] level not found in log")
	}
	if !strings.Contains(logContent, "[INFO]") {
		t.Error("[INFO] level not found in log")
	}
	if !strings.Contains(logContent, "[WARN]") {
		t.Error("[WARN] level not found in log")
	}
	if !strings.Contains(logContent, "[ERROR]") {
		t.Error("[ERROR] level not found in log")
	}

	// Check key=value attributes are present
	if !strings.Contains(logContent, "key=value") {
		t.Error("key=value attributes not found in log")
	}
}

func TestLogLevelFiltering(t *testing.T) {
	logDir := t.TempDir()
	// Set level to INFO (should filter out DEBUG)
	if err := Init("info", logDir, 10); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	defer Close()

	Debug("debug message should not appear")
	Info("info message should appear")

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
	if err := Init("info", logDir, 10); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	defer Close()

	// Get modular loggers
	mainLogger := Get("main")
	dbLogger := Get("database")
	apiLogger := Get("api")

	// Log messages from different modules
	mainLogger.Info("message from main")
	dbLogger.Info("message from database")
	apiLogger.Info("message from api")

	// Read log file
	errorLogPath := filepath.Join(logDir, "error.log")
	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log: %v", err)
	}

	logContent := string(content)

	// Check module names appear in logs
	if !strings.Contains(logContent, "[main]") {
		t.Error("[main] module not found in log")
	}
	if !strings.Contains(logContent, "[database]") {
		t.Error("[database] module not found in log")
	}
	if !strings.Contains(logContent, "[api]") {
		t.Error("[api] module not found in log")
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

	// Manually set up logger with very small maxSize for testing
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	// Create small maxSize (2KB) for testing
	maxSize := int64(2048)
	maxLogSize = maxSize
	logLevel = slog.LevelInfo

	errorLogPath := filepath.Join(logDir, "error.log")
	errorLogFile, err := os.OpenFile(errorLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open error.log: %v", err)
	}

	errorRotated := &rotatedFile{
		file:     errorLogFile,
		filePath: errorLogPath,
		maxSize:  maxSize,
	}
	if info, err := errorLogFile.Stat(); err == nil {
		errorRotated.fileSize = info.Size()
	}

	logFiles = make(map[string]io.WriteCloser)
	logFiles["error"] = errorRotated
	Log = slog.New(&plainTextHandler{w: errorRotated, level: logLevel, module: "main"})

	defer Close()

	// Write large messages to trigger rotation
	// Each message is ~150 bytes, so we need ~15-20 messages to exceed 2KB
	largeMessage := strings.Repeat("This is a large message to test rotation. ", 10)

	for i := 0; i < 20; i++ {
		Info(largeMessage, "iteration", i)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// Check that backup file was created
	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	// Should have error.log + at least one backup file (error.YYYYMMDD_HHMMSS.log)
	if len(files) < 2 {
		t.Errorf("Expected at least 2 files (error.log + backup), got %d", len(files))
		for _, f := range files {
			t.Logf("File: %s", f.Name())
		}
	}

	// Check for backup file naming pattern
	hasBackup := false
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "error.") && strings.Contains(file.Name(), ".log") && file.Name() != "error.log" {
			hasBackup = true
			t.Logf("Found backup file: %s", file.Name())
			break
		}
	}

	if !hasBackup {
		t.Error("No backup file created after rotation")
	}

	// Check that current error.log exists
	errorLogPath = filepath.Join(logDir, "error.log")
	info, err := os.Stat(errorLogPath)
	if err != nil {
		t.Fatalf("error.log does not exist after rotation: %v", err)
	}

	// Current file should exist (size may vary)
	t.Logf("Current error.log size: %d bytes", info.Size())
}

func TestClose(t *testing.T) {
	logDir := t.TempDir()
	if err := Init("info", logDir, 10); err != nil {
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
	if err := Init("info", logDir, 10); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	defer Close()

	Info("test message", "key1", "value1", "key2", "value2")

	errorLogPath := filepath.Join(logDir, "error.log")
	content, err := os.ReadFile(errorLogPath)
	if err != nil {
		t.Fatalf("Failed to read error.log: %v", err)
	}

	logContent := string(content)

	// Check format: YYYY-MM-DD HH:MM:SS.000000 [LEVEL] [module] message key=value
	// Should contain timestamp pattern
	if !strings.Contains(logContent, "[INFO]") {
		t.Error("Log level [INFO] not found")
	}

	if !strings.Contains(logContent, "[main]") {
		t.Error("Module [main] not found")
	}

	if !strings.Contains(logContent, "test message") {
		t.Error("Message not found")
	}

	if !strings.Contains(logContent, "key1=value1") {
		t.Error("key1=value1 not found")
	}

	if !strings.Contains(logContent, "key2=value2") {
		t.Error("key2=value2 not found")
	}

	// Check timestamp format (YYYY-MM-DD)
	if !strings.Contains(logContent, "-") || !strings.Contains(logContent, ":") {
		t.Error("Timestamp format not found in log")
	}
}
