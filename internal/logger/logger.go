package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	Log        *slog.Logger
	logLevel   slog.Level
	logDir     string
	logFiles   map[string]io.WriteCloser
	fileMutex  sync.RWMutex
	maxLogSize int64
)

// rotatedFile - обертка с автоматической ротацией
type rotatedFile struct {
	file      *os.File
	filePath  string
	fileSize  int64
	maxSize   int64
	fileMutex sync.Mutex
}

func (rf *rotatedFile) Write(p []byte) (int, error) {
	rf.fileMutex.Lock()
	defer rf.fileMutex.Unlock()

	// Check if rotation is needed
	if rf.fileSize+int64(len(p)) > rf.maxSize {
		if err := rf.rotate(); err != nil {
			// If rotation fails, still try to write
			n, _ := rf.file.Write(p)
			rf.fileSize += int64(n)
			return n, nil
		}
	}

	n, err := rf.file.Write(p)
	rf.fileSize += int64(n)
	return n, err
}

func (rf *rotatedFile) rotate() error {
	// Close current file
	if err := rf.file.Close(); err != nil {
		return err
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	dir := filepath.Dir(rf.filePath)
	name := filepath.Base(rf.filePath)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	backupPath := filepath.Join(dir, fmt.Sprintf("%s.%s%s", base, timestamp, ext))

	// Rename current file to backup
	if err := os.Rename(rf.filePath, backupPath); err != nil {
		return err
	}

	// Open new file
	f, err := os.OpenFile(rf.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	rf.file = f
	rf.fileSize = 0
	return nil
}

func (rf *rotatedFile) Close() error {
	rf.fileMutex.Lock()
	defer rf.fileMutex.Unlock()
	return rf.file.Close()
}

// plainTextHandler - кастомный handler для slog
type plainTextHandler struct {
	w      io.WriteCloser
	level  slog.Level
	module string
}

func (h *plainTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *plainTextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Format: YYYY-MM-DD HH:MM:SS.000000 [LEVEL] [module] message key=value
	timeStr := r.Time.Format("2006-01-02 15:04:05.000000")
	levelStr := strings.ToUpper(r.Level.String())
	msg := r.Message
	module := h.module

	// Extract additional attributes
	var otherAttrs []string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "module" {
			// Module already set
			return true
		} else if a.Key != slog.TimeKey && a.Key != slog.MessageKey {
			value := fmt.Sprint(a.Value.Any())
			otherAttrs = append(otherAttrs, fmt.Sprintf("%s=%s", a.Key, value))
		}
		return true
	})

	// Build output string
	output := fmt.Sprintf("%s [%s] [%s] %s", timeStr, levelStr, module, msg)
	if len(otherAttrs) > 0 {
		output += " " + strings.Join(otherAttrs, " ")
	}
	output += "\n"

	// Write to file
	switch w := h.w.(type) {
	case *rotatedFile:
		_, err := w.Write([]byte(output))
		return err
	default:
		_, err := io.WriteString(h.w, output)
		return err
	}
}

func (h *plainTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newH := &plainTextHandler{w: h.w, level: h.level, module: h.module}
	for _, a := range attrs {
		if a.Key == "module" {
			newH.module = fmt.Sprint(a.Value.Any())
		}
	}
	return newH
}

func (h *plainTextHandler) WithGroup(name string) slog.Handler {
	return h
}

func init() {
	logFiles = make(map[string]io.WriteCloser)
}

// Init инициализирует систему логирования
func Init(levelStr, dir string, maxFileSizeMB int) error {
	// Create log directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	logDir = dir
	maxLogSize = int64(maxFileSizeMB) * 1024 * 1024

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
	errorLogFile, err := os.OpenFile(errorLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open error.log: %w", err)
	}

	errorRotated := &rotatedFile{
		file:     errorLogFile,
		filePath: errorLogPath,
		maxSize:  maxLogSize,
	}
	if info, err := errorLogFile.Stat(); err == nil {
		errorRotated.fileSize = info.Size()
	}
	logFiles["error"] = errorRotated

	// Create logger
	Log = slog.New(&plainTextHandler{w: errorRotated, level: logLevel, module: "main"})

	return nil
}

// Get возвращает логгер для конкретного модуля
func Get(module string) *slog.Logger {
	if Log == nil {
		// Fallback to stdout if not initialized
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
