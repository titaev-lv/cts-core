package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
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
	Format             string
	Dir                string
	MaxFileSizeMB      int
	MaxBackups         int
	MaxAgeDays         int
	Compress           bool
	ErrorPath          string
	AccessPath         string
	OutRequestPath     string
	WSInPath           string
	WSOutPath          string
	AuditPath          string
	AccessToStdout     bool
	OutRequestToStdout bool
	WSInToStdout       bool
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
	if opts.Format == "" {
		opts.Format = "json"
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
	opts.WSInPath = defaultLogPath(opts.WSInPath, opts.Dir, "ws_in.log")
	opts.WSOutPath = defaultLogPath(opts.WSOutPath, opts.Dir, "ws_out.log")
	opts.AuditPath = defaultLogPath(opts.AuditPath, opts.Dir, "audit.log")

	var err error
	if Log, err = buildLogger("error", opts.ErrorPath, true, opts); err != nil {
		return err
	}
	if AccessLog, err = buildLogger("access", opts.AccessPath, opts.AccessToStdout, opts); err != nil {
		return err
	}
	if OutReqLog, err = buildLogger("out_request", opts.OutRequestPath, opts.OutRequestToStdout, opts); err != nil {
		return err
	}
	if WSAccLog, err = buildLogger("ws_access", opts.WSInPath, opts.WSInToStdout, opts); err != nil {
		return err
	}
	if WSOutLog, err = buildLogger("ws_out", opts.WSOutPath, opts.WSOutToStdout, opts); err != nil {
		return err
	}
	if AuditLog, err = buildLogger("audit", opts.AuditPath, opts.AuditToStdout, opts); err != nil {
		return err
	}

	slog.SetDefault(Log)
	return nil
}

func defaultLogPath(value string, dir string, fallback string) string {
	if value != "" {
		return value
	}
	return filepath.Join(filepath.Clean(dir), fallback)
}

func buildLogger(name string, path string, toStdout bool, opts Options) (*slog.Logger, error) {
	file, err := newRotatingLogFile(path, opts.MaxFileSizeMB, opts.MaxBackups, opts.MaxAgeDays, opts.Compress)
	if err != nil {
		return nil, err
	}
	logFiles[name] = file

	writer := io.Writer(file)
	if toStdout {
		writer = io.MultiWriter(os.Stdout, file)
	}

	handler := newHandler(opts.Format, writer, &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: replaceTimeAttr,
	})
	if name == "access" || name == "ws_access" {
		handler = newHandler(opts.Format, writer, &slog.HandlerOptions{
			Level:       logLevel,
			ReplaceAttr: replaceAccessAttr,
		})
	}
	if name == "audit" {
		handler = newHandler(opts.Format, writer, &slog.HandlerOptions{
			Level:       logLevel,
			ReplaceAttr: replaceAuditAttr,
		})
	}

	return slog.New(handler), nil
}

func newRotatingLogFile(path string, maxSize, maxBackups, maxAge int, compress bool) (*lumberjack.Logger, error) {
	l := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   compress,
	}

	if shouldRotateOnStartup(path) {
		if err := l.Rotate(); err != nil {
			return nil, fmt.Errorf("rotate log on startup %s: %w", path, err)
		}
	}

	return l, nil
}

func shouldRotateOnStartup(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.Mode().IsRegular() {
		return false
	}
	return info.Size() > 0
}

func newHandler(format string, writer io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if strings.EqualFold(format, "text") {
		return slog.NewTextHandler(&textValueOnlyWriter{dst: writer}, opts)
	}
	return slog.NewJSONHandler(writer, opts)
}

type textValueOnlyWriter struct {
	dst io.Writer
}

func (w *textValueOnlyWriter) Write(p []byte) (int, error) {
	if w == nil || w.dst == nil {
		return len(p), nil
	}

	text := string(p)
	lines := strings.SplitAfter(text, "\n")
	var b strings.Builder
	b.Grow(len(text))
	for _, line := range lines {
		if line == "" {
			continue
		}
		hasNewline := strings.HasSuffix(line, "\n")
		trimmed := strings.TrimSuffix(line, "\n")
		b.WriteString(stripTextPrefixKeys(trimmed))
		if hasNewline {
			b.WriteByte('\n')
		}
	}

	if _, err := w.dst.Write([]byte(b.String())); err != nil {
		return 0, err
	}
	return len(p), nil
}

func stripTextPrefixKeys(line string) string {
	if !strings.HasPrefix(line, "time=") {
		return line
	}

	idx := len("time=")
	timeVal, next, ok := readAttrValue(line, idx)
	if !ok {
		return line
	}

	idx = next
	levelVal := ""
	if strings.HasPrefix(line[idx:], "level=") {
		idx += len("level=")
		var levelOK bool
		levelVal, idx, levelOK = readAttrValue(line, idx)
		if !levelOK {
			return line
		}
	}

	if !strings.HasPrefix(line[idx:], "msg=") {
		return line
	}
	idx += len("msg=")
	msgVal, idx, ok := readAttrValue(line, idx)
	if !ok {
		return line
	}
	msgVal = normalizeMsgValue(msgVal)

	rest := strings.TrimLeft(line[idx:], " ")
	if levelVal != "" {
		if rest != "" {
			return timeVal + " " + levelVal + " " + msgVal + " " + rest
		}
		return timeVal + " " + levelVal + " " + msgVal
	}

	if rest != "" {
		return timeVal + " " + msgVal + " " + rest
	}
	return timeVal + " " + msgVal
}

func normalizeMsgValue(raw string) string {
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		unquoted, err := strconv.Unquote(raw)
		if err == nil {
			return unquoted
		}
		return strings.Trim(raw, "\"")
	}
	return raw
}

func readAttrValue(line string, start int) (value string, next int, ok bool) {
	if start >= len(line) {
		return "", start, false
	}

	if line[start] == '"' {
		i := start + 1
		escaped := false
		end := -1
		for i < len(line) {
			ch := line[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if ch == '\\' {
				escaped = true
				i++
				continue
			}
			if ch == '"' {
				end = i + 1
				i = end
				for i < len(line) && line[i] == ' ' {
					i++
				}
				return line[start:end], i, true
			}
			i++
		}
		return "", start, false
	}

	i := start
	for i < len(line) && line[i] != ' ' {
		i++
	}
	val := line[start:i]
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return val, i, true
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
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceAccessAttr})).With("module", module)
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
	if attr.Key == slog.MessageKey || attr.Key == "module" || attr.Key == slog.LevelKey {
		return slog.Attr{}
	}
	return attr
}

func replaceAuditAttr(groups []string, attr slog.Attr) slog.Attr {
	attr = replaceTimeAttr(groups, attr)
	if attr.Key == slog.LevelKey {
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
