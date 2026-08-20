package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRollingLogger(t *testing.T) {
	rl := NewRollingLogger(3)

	rl.Log("INFO", "Message 1")
	rl.Log("PING", "Message 2")
	rl.Log("WARN", "Message 3")

	if rl.Count() != 3 {
		t.Fatalf("expected 3 entries, got %d", rl.Count())
	}

	rl.Log("ERROR", "Message 4")
	if rl.Count() != 3 {
		t.Fatalf("expected 3 entries after overflow, got %d", rl.Count())
	}

	all := rl.GetAll()
	if strings.Contains(all, "Message 1") {
		t.Errorf("expected Message 1 to be dropped, but found in logs")
	}
	if !strings.Contains(all, "Message 2") || !strings.Contains(all, "Message 4") {
		t.Errorf("expected Message 2 and Message 4 to be present in logs: %s", all)
	}

	rl.Clear()
	if rl.Count() != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", rl.Count())
	}
}

func TestRollingLoggerConcurrent(t *testing.T) {
	logger := NewRollingLogger(50)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Log("INFO", "Log message %d", n)
			_ = logger.GetAll()
			_ = logger.Count()
			if n%25 == 0 {
				logger.Clear()
			}
		}(i)
	}
	wg.Wait()
}

func TestGlobalLoggerHelpers(t *testing.T) {
	Info("Test info log %d", 1)
	Ping("Test ping log %s", "C:\\")
	Hotplug("Test hotplug log %s", "E:\\")
	Config("Test config log %s", "interval=45")
	Warn("Test warn log %s", "latency spike")
	Error("Test error log %s", "disk not found")

	logs := DefaultLogger.GetAll()
	if !strings.Contains(logs, "Test info log 1") || !strings.Contains(logs, "Test error log disk not found") {
		t.Errorf("expected logs to contain recorded entries: %s", logs)
	}
}

func TestRollingLoggerEdgeCases(t *testing.T) {
	// Zero/negative max size fallback
	l0 := NewRollingLogger(0)
	if l0.maxSize != MaxLogEntries {
		t.Errorf("expected maxSize %d, got %d", MaxLogEntries, l0.maxSize)
	}

	lNeg := NewRollingLogger(-10)
	if lNeg.maxSize != MaxLogEntries {
		t.Errorf("expected maxSize %d, got %d", MaxLogEntries, lNeg.maxSize)
	}

	// Empty logger output
	emptyLogs := l0.GetAll()
	if emptyLogs != "No logs recorded yet.\n" {
		t.Errorf("unexpected empty logs output: %q", emptyLogs)
	}

	// Clipboard copy test (should execute without panic)
	_ = l0.CopyToClipboard()
}

func TestFileLoggingLifecycleAndContent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_log_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger := NewRollingLogger(10)
	if err := logger.EnableFileLogging(tempDir, 7); err != nil {
		t.Fatalf("EnableFileLogging failed: %v", err)
	}
	defer logger.Close()

	if !logger.IsFileLoggingEnabled() {
		t.Errorf("expected file logging to be enabled")
	}
	if logger.GetLogsDir() != tempDir {
		t.Errorf("expected logs dir %s, got %s", tempDir, logger.GetLogsDir())
	}

	logger.Log("INFO", "Starting test run")
	logger.Log("PING", "Drive %s latency %dms", "E:\\", 12)
	logger.Log("ERROR", "Disk I/O failed: %s", "timeout")

	// Check in-memory buffer
	memLogs := logger.GetAll()
	if !strings.Contains(memLogs, "Starting test run") || !strings.Contains(memLogs, "Drive E:\\ latency 12ms") {
		t.Errorf("missing logs in memory buffer: %s", memLogs)
	}

	// Check physical file on disk
	todayStr := time.Now().Format("2006-01-02")
	expectedFileName := filepath.Join(tempDir, fmt.Sprintf("drivepulse-%s.log", todayStr))
	data, err := os.ReadFile(expectedFileName)
	if err != nil {
		t.Fatalf("failed to read log file %s: %v", expectedFileName, err)
	}

	fileContent := string(data)
	if !strings.Contains(fileContent, "[INFO] Starting test run") ||
		!strings.Contains(fileContent, "[PING] Drive E:\\ latency 12ms") ||
		!strings.Contains(fileContent, "[ERROR] Disk I/O failed: timeout") {
		t.Errorf("file content missing expected logs: %s", fileContent)
	}
}

func TestFileLoggingPruneOldLogs7Days(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_prune_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	now := time.Now()
	daysOffsets := []int{0, 2, 6, 7, 8, 15, 30}
	for _, offset := range daysOffsets {
		d := now.AddDate(0, 0, -offset)
		fname := filepath.Join(tempDir, fmt.Sprintf("drivepulse-%s.log", d.Format("2006-01-02")))
		if err := os.WriteFile(fname, []byte(fmt.Sprintf("log content for offset %d", offset)), 0644); err != nil {
			t.Fatalf("failed to write test log file: %v", err)
		}
	}

	keepFile := filepath.Join(tempDir, "other_file.txt")
	_ = os.WriteFile(keepFile, []byte("preserve me"), 0644)

	logger := NewRollingLogger(10)
	if err := logger.EnableFileLogging(tempDir, 7); err != nil {
		t.Fatalf("EnableFileLogging failed: %v", err)
	}
	defer logger.Close()

	for _, offset := range daysOffsets {
		d := now.AddDate(0, 0, -offset)
		fname := filepath.Join(tempDir, fmt.Sprintf("drivepulse-%s.log", d.Format("2006-01-02")))
		_, err := os.Stat(fname)
		if offset <= 7 {
			if os.IsNotExist(err) {
				t.Errorf("expected file for offset %d days to be preserved, but was deleted", offset)
			}
		} else {
			if !os.IsNotExist(err) {
				t.Errorf("expected file for offset %d days to be deleted on startup, but still exists", offset)
			}
		}
	}

	if _, err := os.Stat(keepFile); os.IsNotExist(err) {
		t.Errorf("expected non-log file to be preserved")
	}

	extraOldFile := filepath.Join(tempDir, fmt.Sprintf("drivepulse-%s.log", now.AddDate(0, 0, -12).Format("2006-01-02")))
	if err := os.WriteFile(extraOldFile, []byte("extra old log"), 0644); err != nil {
		t.Fatalf("failed to write extra old log: %v", err)
	}
	deleted := logger.PruneOldLogs(7)
	if deleted != 1 {
		t.Errorf("expected 1 deleted file during explicit prune, got %d", deleted)
	}
	if _, err := os.Stat(extraOldFile); !os.IsNotExist(err) {
		t.Errorf("expected extra old file to be deleted")
	}
}

func TestFileLoggingDateRotation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_rotation_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger := NewRollingLogger(10)
	if err := logger.EnableFileLogging(tempDir, 7); err != nil {
		t.Fatalf("EnableFileLogging failed: %v", err)
	}
	defer logger.Close()

	logger.Log("INFO", "Log for today")

	todayStr := time.Now().Format("2006-01-02")
	todayPath := filepath.Join(tempDir, fmt.Sprintf("drivepulse-%s.log", todayStr))
	if _, err := os.Stat(todayPath); err != nil {
		t.Fatalf("expected today's log file to exist: %v", err)
	}

	logger.mu.Lock()
	logger.currentDate = "2000-01-01"
	logger.mu.Unlock()

	logger.Log("PING", "Log triggering rotation check")

	data, err := os.ReadFile(todayPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "Log for today") || !strings.Contains(string(data), "Log triggering rotation check") {
		t.Errorf("expected log file to contain both messages: %s", string(data))
	}
}

func TestFileLoggingFallbackMemoryOnly(t *testing.T) {
	logger := NewRollingLogger(10)
	err := logger.EnableFileLogging("", 7)
	if err == nil {
		t.Errorf("expected EnableFileLogging with empty dir to return error")
	}
	if logger.IsFileLoggingEnabled() {
		t.Errorf("expected IsFileLoggingEnabled to be false after failed init")
	}

	logger.Log("WARN", "Memory only test message")
	if !strings.Contains(logger.GetAll(), "Memory only test message") {
		t.Errorf("expected in-memory logs to still be recorded")
	}
}

func TestRollingLoggerConcurrentWithFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_concurrent_file_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger := NewRollingLogger(50)
	if err := logger.EnableFileLogging(tempDir, 7); err != nil {
		t.Fatalf("EnableFileLogging failed: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				logger.Log("PING", "Goroutine %d message %d", id, j)
				_ = logger.GetAll()
				_ = logger.Count()
				_ = logger.IsFileLoggingEnabled()
			}
		}(i)
	}
	wg.Wait()

	if logger.Count() == 0 {
		t.Errorf("expected non-zero entries in logger")
	}
}

func TestRollingLoggerCloseLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_close_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger := NewRollingLogger(10)
	if err := logger.EnableFileLogging(tempDir, 7); err != nil {
		t.Fatalf("EnableFileLogging failed: %v", err)
	}

	logger.Log("INFO", "Pre-close log")
	if !logger.IsFileLoggingEnabled() {
		t.Errorf("expected file logging to be enabled")
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if logger.IsFileLoggingEnabled() {
		t.Errorf("expected file logging to be disabled after Close()")
	}

	if err := logger.Close(); err != nil {
		t.Errorf("expected second Close() to be safe and return nil, got %v", err)
	}

	logger.Log("INFO", "Post-close log")
	if !strings.Contains(logger.GetAll(), "Post-close log") {
		t.Errorf("expected in-memory logs to record after file logger is closed")
	}
}
