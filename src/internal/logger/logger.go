package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

const (
	LogsDirName          = "logs"
	DefaultRetentionDays = 7
	LogFilePrefix        = "drivepulse-"
	LogFileExt           = ".log"
	MaxLogEntries        = 500
)

type LogEntry struct {
	Timestamp time.Time
	Category  string
	Message   string
}

func (e LogEntry) Formatted() string {
	return fmt.Sprintf("[%s] [%s] %s", e.Timestamp.Format("2006-01-02 15:04:05"), e.Category, e.Message)
}

type RollingLogger struct {
	mu            sync.RWMutex
	entries       []LogEntry
	maxSize       int
	logsDir       string
	retentionDays int
	currentDate   string
	currentFile   *os.File
	fileLoggingOk bool
}

var DefaultLogger = NewRollingLogger(MaxLogEntries)

func NewRollingLogger(maxSize int) *RollingLogger {
	if maxSize <= 0 {
		maxSize = MaxLogEntries
	}
	return &RollingLogger{
		entries:       make([]LogEntry, 0, maxSize),
		maxSize:       maxSize,
		retentionDays: DefaultRetentionDays,
	}
}

// EnableFileLogging configures daily rolling file logging with automatic retention pruning.
func (l *RollingLogger) EnableFileLogging(logsDir string, retentionDays int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}

	if err := os.MkdirAll(logsDir, 0755); err != nil {
		l.fileLoggingOk = false
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	l.logsDir = logsDir
	l.retentionDays = retentionDays
	l.fileLoggingOk = true

	now := time.Now()
	if err := l.rotateFileLocked(now); err != nil {
		l.fileLoggingOk = false
		return fmt.Errorf("failed to open daily log file: %w", err)
	}

	return nil
}

// IsFileLoggingEnabled checks whether persistent file logging is currently active.
func (l *RollingLogger) IsFileLoggingEnabled() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.fileLoggingOk
}

// GetLogsDir returns the configured logs directory.
func (l *RollingLogger) GetLogsDir() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logsDir
}

// rotateFileLocked rotates the active log file to the current date and executes retention pruning.
func (l *RollingLogger) rotateFileLocked(now time.Time) error {
	if l.currentFile != nil {
		_ = l.currentFile.Close()
		l.currentFile = nil
	}

	if l.logsDir == "" {
		return fmt.Errorf("logs directory not configured")
	}

	if err := os.MkdirAll(l.logsDir, 0755); err != nil {
		return err
	}

	dateStr := now.Format("2006-01-02")
	fileName := fmt.Sprintf("%s%s%s", LogFilePrefix, dateStr, LogFileExt)
	filePath := filepath.Join(l.logsDir, fileName)

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.currentDate = dateStr
	l.currentFile = f

	// Prune old logs on daily rotation
	l.pruneOldLogsLocked(now)
	return nil
}

// pruneOldLogsLocked removes daily log files older than retentionDays.
func (l *RollingLogger) pruneOldLogsLocked(now time.Time) int {
	if l.logsDir == "" {
		return 0
	}
	retention := l.retentionDays
	if retention <= 0 {
		retention = DefaultRetentionDays
	}

	entries, err := os.ReadDir(l.logsDir)
	if err != nil {
		return 0
	}

	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -retention)
	deletedCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, LogFilePrefix) || !strings.HasSuffix(name, LogFileExt) {
			continue
		}

		datePart := strings.TrimSuffix(strings.TrimPrefix(name, LogFilePrefix), LogFileExt)
		parsedDate, err := time.ParseInLocation("2006-01-02", datePart, now.Location())
		if err == nil {
			if parsedDate.Before(cutoff) {
				if err := os.Remove(filepath.Join(l.logsDir, name)); err == nil {
					deletedCount++
				}
			}
		} else {
			// Fallback: check file ModTime if date couldn't be parsed from filename
			if info, err := entry.Info(); err == nil {
				if info.ModTime().Before(cutoff) {
					if err := os.Remove(filepath.Join(l.logsDir, name)); err == nil {
						deletedCount++
					}
				}
			}
		}
	}
	return deletedCount
}

// PruneOldLogs triggers log retention cleanup.
func (l *RollingLogger) PruneOldLogs(retentionDays int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if retentionDays > 0 {
		l.retentionDays = retentionDays
	}
	return l.pruneOldLogsLocked(time.Now())
}

// Close closes the active log file handle and disables further file logging.
func (l *RollingLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.fileLoggingOk = false
	if l.currentFile != nil {
		err := l.currentFile.Close()
		l.currentFile = nil
		return err
	}
	return nil
}

func (l *RollingLogger) Log(category string, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	now := time.Now()
	entry := LogEntry{
		Timestamp: now,
		Category:  strings.ToUpper(strings.TrimSpace(category)),
		Message:   msg,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// In-memory ring buffer
	if len(l.entries) >= l.maxSize {
		copy(l.entries, l.entries[1:])
		l.entries[len(l.entries)-1] = entry
	} else {
		l.entries = append(l.entries, entry)
	}

	// Persistent daily log file
	if l.fileLoggingOk {
		dateStr := now.Format("2006-01-02")
		if dateStr != l.currentDate || l.currentFile == nil {
			_ = l.rotateFileLocked(now)
		}

		if l.currentFile != nil {
			line := entry.Formatted() + "\n"
			if _, err := l.currentFile.WriteString(line); err != nil {
				_ = l.currentFile.Close()
				l.currentFile = nil
			}
		}
	}
}

func (l *RollingLogger) GetAll() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.entries) == 0 {
		return "No logs recorded yet.\n"
	}

	var sb strings.Builder
	sb.WriteString("=== DrivePulse Session Logs ===\n")
	for _, e := range l.entries {
		sb.WriteString(e.Formatted())
		sb.WriteByte('\n')
	}
	sb.WriteString("=== End of Logs ===\n")
	return sb.String()
}

func (l *RollingLogger) CopyToClipboard() error {
	logs := l.GetAll()
	return clipboard.WriteAll(logs)
}

func (l *RollingLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}

func (l *RollingLogger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

func Info(format string, args ...interface{})    { DefaultLogger.Log("INFO", format, args...) }
func Ping(format string, args ...interface{})    { DefaultLogger.Log("PING", format, args...) }
func Hotplug(format string, args ...interface{}) { DefaultLogger.Log("HOTPLUG", format, args...) }
func Config(format string, args ...interface{})  { DefaultLogger.Log("CONFIG", format, args...) }
func Warn(format string, args ...interface{})    { DefaultLogger.Log("WARN", format, args...) }
func Error(format string, args ...interface{})   { DefaultLogger.Log("ERROR", format, args...) }
