package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Log       *slog.Logger
	logLevel  slog.Level
	logDir    string
	logFiles  map[string]io.WriteCloser
	fileMutex sync.RWMutex
)

func init() {
	logFiles = make(map[string]io.WriteCloser)
}

// Init инициализирует систему логирования
func Init(levelStr, dir string, maxFileSizeMB int, maxBackups int, maxAgeDays int, compress bool) error {
	if err := validateLogDir(dir); err != nil {
		return err
	}

	logDir = dir
	if maxFileSizeMB <= 0 {
		maxFileSizeMB = 100
	}
	if maxBackups <= 0 {
		maxBackups = 10
	}
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}

	// Parse log level
	switch strings.ToLower(levelStr) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// Initialize error.log
	errorLogPath := filepath.Join(filepath.Clean(dir), "error.log")
	errorLogFile := &lumberjack.Logger{
		Filename:   errorLogPath,
		MaxSize:    maxFileSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	logFiles["error"] = errorLogFile

	writer := io.MultiWriter(os.Stdout, errorLogFile)
	opts := &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: replaceTimeAttr,
	}

	// Create logger (JSON)
	Log = slog.New(slog.NewJSONHandler(writer, opts))
	slog.SetDefault(Log)

	return nil
}

// Get возвращает логгер для конкретного модуля
func Get(module string) *slog.Logger {
	if Log == nil {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceTimeAttr}))
	}
	return Log.With("module", module)
}

// Debug logs debug message to error.log
func Debug(msg string, args ...any) {
	if Log != nil {
		Log.Debug(msg, args...)
	}
}

// Info logs info message to error.log
func Info(msg string, args ...any) {
	if Log != nil {
		Log.Info(msg, args...)
	}
}

// Warn logs warning message to error.log
func Warn(msg string, args ...any) {
	if Log != nil {
		Log.Warn(msg, args...)
	}
}

// Error logs error message to error.log
func Error(msg string, args ...any) {
	if Log != nil {
		Log.Error(msg, args...)
	}
}

// Close закрывает все открытые файлы логов
func Close() error {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	var lastErr error
	for name, f := range logFiles {
		if err := f.Close(); err != nil {
			lastErr = err
		}
		delete(logFiles, name)
	}
	return lastErr
}

// GetLevel возвращает текущий уровень логирования
func GetLevel() slog.Level {
	return logLevel
}

// GetLogDir возвращает директорию логов
func GetLogDir() string {
	return logDir
}

func replaceTimeAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key != slog.TimeKey {
		return attr
	}
	if t, ok := attr.Value.Any().(time.Time); ok {
		attr.Value = slog.StringValue(t.UTC().Format("2006-01-02T15:04:05.000000Z"))
	}
	return attr
}

func validateLogDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("log directory is empty")
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create log dir %s: %w", dir, err)
	}

	file, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return fmt.Errorf("create write test in %s: %w", dir, err)
	}
	name := file.Name()
	if _, err := file.WriteString("test"); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write test in %s: %w", dir, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close write test in %s: %w", dir, err)
	}

	rotated := name + ".rotate"
	if err := os.Rename(name, rotated); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("rename write test in %s: %w", dir, err)
	}
	if err := os.Remove(rotated); err != nil {
		return fmt.Errorf("cleanup write test in %s: %w", dir, err)
	}

	return nil
}
