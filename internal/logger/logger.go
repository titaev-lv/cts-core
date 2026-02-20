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
	AccessLog *slog.Logger
	OutReqLog *slog.Logger
	WSAccLog  *slog.Logger
	WSOutLog  *slog.Logger
	AuditLog  *slog.Logger
	logLevel  slog.Level
	logDir    string
	logFiles  map[string]io.WriteCloser
	fileMutex sync.RWMutex
)

func init() {
	logFiles = make(map[string]io.WriteCloser)
}

// Init инициализирует систему логирования
type Options struct {
	Level              string
	Dir                string
	MaxFileSizeMB      int
	MaxBackups         int
	MaxAgeDays         int
	Compress           bool
	ErrorPath          string
	AccessPath         string
	OutRequestPath     string
	WSAccessPath       string
	WSOutPath          string
	AuditPath          string
	AccessToStdout     bool
	OutRequestToStdout bool
	WSAccessToStdout   bool
	WSOutToStdout      bool
	AuditToStdout      bool
}

func Init(levelStr, dir string, maxFileSizeMB int, maxBackups int, maxAgeDays int, compress bool) error {
	return InitWithOptions(Options{
		Level:         levelStr,
		Dir:           dir,
		MaxFileSizeMB: maxFileSizeMB,
		MaxBackups:    maxBackups,
		MaxAgeDays:    maxAgeDays,
		Compress:      compress,
	})
}

// InitWithOptions initializes all CTS-Core log streams.
func InitWithOptions(opts Options) error {
	if err := validateLogDir(opts.Dir); err != nil {
		return err
	}

	logDir = opts.Dir
	if opts.MaxFileSizeMB <= 0 {
		opts.MaxFileSizeMB = 100
	}
	if opts.MaxBackups <= 0 {
		opts.MaxBackups = 10
	}
	if opts.MaxAgeDays <= 0 {
		opts.MaxAgeDays = 30
	}

	// Parse log level
	switch strings.ToLower(opts.Level) {
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

	opts.ErrorPath = defaultLogPath(opts.ErrorPath, opts.Dir, "error.log")
	opts.AccessPath = defaultLogPath(opts.AccessPath, opts.Dir, "access.log")
	opts.OutRequestPath = defaultLogPath(opts.OutRequestPath, opts.Dir, "out_request.log")
	opts.WSAccessPath = defaultLogPath(opts.WSAccessPath, opts.Dir, "ws_access.log")
	opts.WSOutPath = defaultLogPath(opts.WSOutPath, opts.Dir, "ws_out.log")
	opts.AuditPath = defaultLogPath(opts.AuditPath, opts.Dir, "audit.log")

	Log = buildLogger("error", opts.ErrorPath, true, opts)
	AccessLog = buildLogger("access", opts.AccessPath, opts.AccessToStdout, opts)
	OutReqLog = buildLogger("out_request", opts.OutRequestPath, opts.OutRequestToStdout, opts)
	WSAccLog = buildLogger("ws_access", opts.WSAccessPath, opts.WSAccessToStdout, opts)
	WSOutLog = buildLogger("ws_out", opts.WSOutPath, opts.WSOutToStdout, opts)
	AuditLog = buildLogger("audit", opts.AuditPath, opts.AuditToStdout, opts)

	slog.SetDefault(Log)
	return nil
}

func defaultLogPath(value string, dir string, fallback string) string {
	if value != "" {
		return value
	}
	return filepath.Join(filepath.Clean(dir), fallback)
}

func buildLogger(name string, path string, toStdout bool, opts Options) *slog.Logger {
	file := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    opts.MaxFileSizeMB,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAgeDays,
		Compress:   opts.Compress,
	}
	logFiles[name] = file

	writer := io.Writer(file)
	if toStdout {
		writer = io.MultiWriter(os.Stdout, file)
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: replaceTimeAttr,
	})
	if name == "access" {
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level:       logLevel,
			ReplaceAttr: replaceAccessAttr,
		})
	}

	return slog.New(handler)
}

// Get возвращает логгер для конкретного модуля
func Get(module string) *slog.Logger {
	if Log == nil {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceTimeAttr}))
	}
	return Log.With("module", module)
}

func GetAccess(_ string) *slog.Logger {
	if AccessLog == nil {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceAccessAttr}))
	}
	return AccessLog
}

func GetOutRequest(module string) *slog.Logger {
	if OutReqLog == nil {
		return Get(module)
	}
	return OutReqLog.With("module", module)
}

func GetWSAccess(module string) *slog.Logger {
	if WSAccLog == nil {
		return Get(module)
	}
	return WSAccLog.With("module", module)
}

func GetWSOut(module string) *slog.Logger {
	if WSOutLog == nil {
		return Get(module)
	}
	return WSOutLog.With("module", module)
}

func GetAudit(module string) *slog.Logger {
	if AuditLog == nil {
		return Get(module)
	}
	return AuditLog.With("module", module)
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

func replaceAccessAttr(groups []string, attr slog.Attr) slog.Attr {
	attr = replaceTimeAttr(groups, attr)
	if attr.Key == slog.MessageKey || attr.Key == "module" {
		return slog.Attr{}
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
