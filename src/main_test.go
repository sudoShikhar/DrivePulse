package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConfigManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_config_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	cfg := mgr.Get()
	if !cfg.MasterEnabled {
		t.Errorf("expected default MasterEnabled = true")
	}
	if cfg.IntervalSeconds != 45 {
		t.Errorf("expected default IntervalSeconds = 45, got %d", cfg.IntervalSeconds)
	}

	if err := mgr.SetDriveSelected("E:\\", true); err != nil {
		t.Fatalf("failed to set drive E: %v", err)
	}
	if err := mgr.SetDriveSelected("F:\\", true); err != nil {
		t.Fatalf("failed to set drive F: %v", err)
	}

	if !mgr.IsDriveSelected("E:\\") || !mgr.IsDriveSelected("F:\\") {
		t.Errorf("expected E:\\ and F:\\ to be selected")
	}
	if mgr.IsDriveSelected("C:\\") {
		t.Errorf("expected C:\\ to not be selected")
	}

	mgr2, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to reload manager: %v", err)
	}
	cfg2 := mgr2.Get()
	if len(cfg2.SelectedDrives) != 2 {
		t.Errorf("expected 2 selected drives after reload, got %d", len(cfg2.SelectedDrives))
	}

	if err := mgr2.SetDriveSelected("E:\\", false); err != nil {
		t.Fatalf("failed to unselect drive: %v", err)
	}
	if mgr2.IsDriveSelected("E:\\") {
		t.Errorf("expected E:\\ to be unselected")
	}
}

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

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{16 * 1024 * 1024 * 1024 * 1024, "16.0 TB"},
	}

	for _, tt := range tests {
		res := FormatBytes(tt.bytes)
		if res != tt.expected {
			t.Errorf("FormatBytes(%d) = %q, expected %q", tt.bytes, res, tt.expected)
		}
	}
}

func TestDetector(t *testing.T) {
	drives, err := DetectDrives()
	if err != nil {
		t.Fatalf("DetectDrives failed: %v", err)
	}

	if len(drives) == 0 {
		t.Logf("Warning: 0 drives detected on this host")
	} else {
		for _, d := range drives {
			t.Logf("Detected: %s | Label: %s | Type: %s | FS: %s | Ready: %v | Total: %s",
				d.Path, d.Label, d.Type, d.FileSystem, d.IsReady, FormatBytes(d.TotalBytes))
		}
	}
}

func TestPingDrive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_engine_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	res := PingDrive(tempDir)
	if !res.Success {
		t.Fatalf("PingDrive failed: %v", res.Error)
	}
	if res.Latency <= 0 {
		t.Errorf("expected positive latency, got %v", res.Latency)
	}

	pingFile := filepath.Join(tempDir, PingFileName)
	content, err := os.ReadFile(pingFile)
	if err != nil {
		t.Fatalf("failed to read ping file: %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("ping file is empty")
	}
}

func TestEngineLoop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_engine_loop_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 1, true)
	eng.Start()
	defer eng.Stop()

	time.Sleep(100 * time.Millisecond)

	status, lastPing := eng.GetStatus()
	if lastPing.IsZero() {
		t.Errorf("expected lastPing to be set")
	}
	if s, ok := status[filepath.Clean(tempDir)]; !ok || !s.Success {
		t.Errorf("expected drive status to be success, got: %+v", s)
	}

	eng.TriggerPingNow()
	time.Sleep(100 * time.Millisecond)
}

func TestEngineMultiDriveResilience(t *testing.T) {
	tempDir1, err := os.MkdirTemp("", "drivepulse_resilience_test_1_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	tempDir2, err := os.MkdirTemp("", "drivepulse_resilience_test_2_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	nonExistent := filepath.Join(tempDir1, "non_existent_drive_folder")

	eng := NewEngine([]string{tempDir1, tempDir2, nonExistent}, 1, true)
	eng.performPings()

	status, lastPing := eng.GetStatus()
	if lastPing.IsZero() {
		t.Errorf("expected lastPing to be recorded")
	}
	if len(status) != 3 {
		t.Errorf("expected 3 drive status results, got %d", len(status))
	}
	if s1, ok := status[filepath.Clean(tempDir1)]; !ok || !s1.Success {
		t.Errorf("expected tempDir1 to succeed, got: %+v", s1)
	}
	if s2, ok := status[filepath.Clean(tempDir2)]; !ok || !s2.Success {
		t.Errorf("expected tempDir2 to succeed, got: %+v", s2)
	}
	if s3, ok := status[filepath.Clean(nonExistent)]; !ok || s3.Success {
		t.Errorf("expected nonExistent to fail gracefully, got: %+v", s3)
	}
}

func TestConfigCorruptedFileResilience(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_corrupt_config_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{ "master_enabled": true, "interval_seconds": NOT_A_NUMBER }`), 0644); err != nil {
		t.Fatalf("failed to write malformed config: %v", err)
	}

	_, err = NewConfigManager(configPath)
	if err == nil {
		t.Errorf("expected error on malformed JSON config, got nil")
	}
}

func TestConfigIntervalValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_interval_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	for _, interval := range AllowedIntervals {
		if err := mgr.SetInterval(interval); err != nil {
			t.Errorf("SetInterval(%d) failed: %v", interval, err)
		}
		if mgr.Get().IntervalSeconds != interval {
			t.Errorf("expected IntervalSeconds = %d, got %d", interval, mgr.Get().IntervalSeconds)
		}
	}
}

func TestConfigConcurrentAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_concurrent_config_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			drive := fmt.Sprintf("X%d:\\", idx%5)
			_ = mgr.SetDriveSelected(drive, idx%2 == 0)
			_ = mgr.SetMasterEnabled(idx%3 == 0)
			_ = mgr.SetAutostart(idx%2 == 0)
			_ = mgr.Get()
			_ = mgr.IsDriveSelected(drive)
		}(i)
	}
	wg.Wait()
}

func TestEngineDynamicConfiguration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_dynamic_engine_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 30, true)
	eng.Start()
	defer eng.Stop()

	eng.SetDrives([]string{tempDir, filepath.Join(tempDir, "sub")})
	eng.SetInterval(60)
	eng.SetEnabled(false)
	eng.SetEnabled(true)
	eng.TriggerPingNow()

	time.Sleep(100 * time.Millisecond)

	status, lastPing := eng.GetStatus()
	if lastPing.IsZero() {
		t.Errorf("expected lastPing to be recorded after dynamic operations")
	}
	if len(status) == 0 {
		t.Errorf("expected drive status to be populated")
	}
}

func TestEngineStopStartIdempotency(t *testing.T) {
	eng := NewEngine(nil, 45, true)
	eng.Start()
	eng.Start()

	eng.Stop()
	eng.Stop()
}

func TestEngineDisabledState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_disabled_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 45, false)
	eng.performPings()

	status, lastPing := eng.GetStatus()
	if !lastPing.IsZero() {
		t.Errorf("expected lastPing to remain zero when engine is disabled")
	}
	if len(status) != 0 {
		t.Errorf("expected no status entries when engine is disabled, got %d", len(status))
	}
}

func TestPingDriveConcurrentSamePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_concurrent_ping_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := PingDrive(tempDir)
			if !res.Success {
				t.Errorf("concurrent ping failed: %v", res.Error)
			}
		}()
	}
	wg.Wait()
}

func TestPingFileContentFormat(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_ping_format_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	res := PingDrive(tempDir)
	if !res.Success {
		t.Fatalf("PingDrive failed: %v", res.Error)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, PingFileName))
	if err != nil {
		t.Fatalf("failed to read ping file: %v", err)
	}

	timestampStr := strings.TrimSpace(string(content))
	parsedTime, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		t.Fatalf("failed to parse timestamp %q as RFC3339Nano: %v", timestampStr, err)
	}

	if time.Since(parsedTime) > 5*time.Second {
		t.Errorf("parsed timestamp %v is too far from current time", parsedTime)
	}
}

func TestSingleInstanceAcquisition(t *testing.T) {
	testMutex := fmt.Sprintf("Local\\DrivePulse_Test_Mutex_%d", time.Now().UnixNano())
	h1, err := AcquireSingleInstanceNamed(testMutex)
	if err != nil {
		t.Fatalf("first AcquireSingleInstanceNamed failed: %v", err)
	}
	defer h1.Release()

	// Second acquisition with same name should fail with ErrAlreadyRunning
	h2, err := AcquireSingleInstanceNamed(testMutex)
	if !errors.Is(err, ErrAlreadyRunning) {
		if h2 != nil {
			h2.Release()
		}
		t.Errorf("expected ErrAlreadyRunning on second acquisition, got: %v", err)
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

func TestDriveInfoDisplayName(t *testing.T) {
	tests := []struct {
		info     DriveInfo
		contains []string
	}{
		{
			info:     DriveInfo{Path: "E:\\", Label: "Elements", Type: DriveTypeFixed, TotalBytes: 16 * 1024 * 1024 * 1024 * 1024},
			contains: []string{"E:\\", "Elements", "16.0 TB"},
		},
		{
			info:     DriveInfo{Path: "F:\\", Label: "", Type: DriveTypeRemovable, TotalBytes: 8 * 1024 * 1024 * 1024},
			contains: []string{"F:\\", "Removable", "8.0 GB"},
		},
		{
			info:     DriveInfo{Path: "G:\\", Label: "", Type: DriveTypeFixed, TotalBytes: 0},
			contains: []string{"G:\\", "Fixed"},
		},
	}

	for _, tt := range tests {
		name := tt.info.DisplayName()
		for _, sub := range tt.contains {
			if !strings.Contains(name, sub) {
				t.Errorf("DisplayName() = %q, expected to contain %q", name, sub)
			}
		}
	}
}

func TestEmbeddedIconsIntegrity(t *testing.T) {
	if len(iconActiveICO) == 0 || len(iconActivePNG) == 0 {
		t.Errorf("expected active icons to be non-empty")
	}
	if len(iconDisabledICO) == 0 || len(iconDisabledPNG) == 0 {
		t.Errorf("expected disabled icons to be non-empty")
	}
	if len(iconWarningICO) == 0 || len(iconWarningPNG) == 0 {
		t.Errorf("expected warning icons to be non-empty")
	}

	pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.HasPrefix(iconActivePNG, pngMagic) {
		t.Errorf("iconActivePNG has invalid PNG magic bytes")
	}
	if !bytes.HasPrefix(iconDisabledPNG, pngMagic) {
		t.Errorf("iconDisabledPNG has invalid PNG magic bytes")
	}
	if !bytes.HasPrefix(iconWarningPNG, pngMagic) {
		t.Errorf("iconWarningPNG has invalid PNG magic bytes")
	}

	icoMagic := []byte{0x00, 0x00, 0x01, 0x00}
	if !bytes.HasPrefix(iconActiveICO, icoMagic) {
		t.Errorf("iconActiveICO has invalid ICO magic bytes")
	}
	if !bytes.HasPrefix(iconDisabledICO, icoMagic) {
		t.Errorf("iconDisabledICO has invalid ICO magic bytes")
	}
	if !bytes.HasPrefix(iconWarningICO, icoMagic) {
		t.Errorf("iconWarningICO has invalid ICO magic bytes")
	}

	if len(getActiveIcon()) == 0 {
		t.Errorf("getActiveIcon() returned empty slice")
	}
	if len(getDisabledIcon()) == 0 {
		t.Errorf("getDisabledIcon() returned empty slice")
	}
	if len(getWarningIcon()) == 0 {
		t.Errorf("getWarningIcon() returned empty slice")
	}
}

func TestGlobalLoggerHelpers(t *testing.T) {
	logInfo("Test info log %d", 1)
	logPing("Test ping log %s", "C:\\")
	logHotplug("Test hotplug log %s", "E:\\")
	logConfig("Test config log %s", "interval=45")
	logWarn("Test warn log %s", "latency spike")
	logError("Test error log %s", "disk not found")

	logs := defaultLogger.GetAll()
	if !strings.Contains(logs, "Test info log 1") || !strings.Contains(logs, "Test error log disk not found") {
		t.Errorf("expected logs to contain recorded entries: %s", logs)
	}
}

func TestDefaultConfigAndPaths(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.MasterEnabled || cfg.IntervalSeconds != 45 || !cfg.Autostart {
		t.Errorf("unexpected default config values: %+v", cfg)
	}

	configPath, err := GetDefaultConfigPath()
	if err != nil {
		t.Fatalf("GetDefaultConfigPath failed: %v", err)
	}
	if !strings.Contains(configPath, "DrivePulse") || !strings.HasSuffix(configPath, "config.json") {
		t.Errorf("unexpected default config path: %s", configPath)
	}
}

func TestAutostartInspection(t *testing.T) {
	_, err := IsAutostartEnabled()
	if err != nil {
		t.Logf("IsAutostartEnabled returned error on test runner: %v", err)
	}
}

func TestCleanDrives(t *testing.T) {
	input := []string{"", "  ", "E:\\", "e:\\", "F:\\data", "C:", "C:\\"}
	cleaned := cleanDrives(input)

	if len(cleaned) == 0 {
		t.Fatalf("expected non-empty cleaned drives")
	}

	// Ensure no empty or whitespace strings remain
	for _, d := range cleaned {
		if strings.TrimSpace(d) == "" {
			t.Errorf("expected no empty/whitespace entries in cleaned drives: %v", cleaned)
		}
	}
}

func TestPingDriveEmptyAndInvalid(t *testing.T) {
	// Empty path should return error immediately without writing to cwd
	res := PingDrive("")
	if res.Success {
		t.Errorf("expected PingDrive('') to fail")
	}
	if res.Error == nil {
		t.Errorf("expected non-nil error for empty drive path")
	}

	// Non-existent path
	res2 := PingDrive("Z:\\NonExistent_DrivePulse_Dir_12345")
	if res2.Success {
		t.Errorf("expected non-existent drive ping to fail")
	}
}

func TestConfigManagerCaseInsensitivity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_case_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	if err := mgr.SetDriveSelected("E:\\", true); err != nil {
		t.Fatalf("SetDriveSelected failed: %v", err)
	}

	// Lowercase lookup should match on Windows
	if !mgr.IsDriveSelected("e:\\") {
		t.Errorf("expected IsDriveSelected('e:\\') to be true")
	}

	// Unselecting with lowercase should work
	if err := mgr.SetDriveSelected("e:\\", false); err != nil {
		t.Fatalf("SetDriveSelected(false) failed: %v", err)
	}
	if mgr.IsDriveSelected("E:\\") {
		t.Errorf("expected E:\\ to be unselected")
	}
}

func TestRollingLoggerEdgeCases(t *testing.T) {
	// Zero/negative max size fallback
	l0 := NewRollingLogger(0)
	if l0.maxSize != maxLogEntries {
		t.Errorf("expected maxSize %d, got %d", maxLogEntries, l0.maxSize)
	}

	lNeg := NewRollingLogger(-10)
	if lNeg.maxSize != maxLogEntries {
		t.Errorf("expected maxSize %d, got %d", maxLogEntries, lNeg.maxSize)
	}

	// Empty logger output
	emptyLogs := l0.GetAll()
	if emptyLogs != "No logs recorded yet.\n" {
		t.Errorf("unexpected empty logs output: %q", emptyLogs)
	}

	// Clipboard copy test (should execute without panic)
	_ = l0.CopyToClipboard()
}

func TestEngineRestartability(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_restart_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 1, true)
	eng.Start()
	time.Sleep(50 * time.Millisecond)
	eng.Stop()

	// Restart engine
	eng.Start()
	time.Sleep(50 * time.Millisecond)
	eng.Stop()
}

func TestEngineSettersWhenStopped(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_stopped_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine(nil, 45, true)
	// Modify properties while stopped
	eng.SetDrives([]string{tempDir})
	eng.SetInterval(90)
	eng.SetEnabled(false)

	// Verify status reflect changes
	eng.mu.RLock()
	drives := eng.drives
	interval := eng.interval
	enabled := eng.enabled
	eng.mu.RUnlock()

	if len(drives) != 1 || drives[0] != filepath.Clean(tempDir) {
		t.Errorf("expected drives to be updated while stopped: %v", drives)
	}
	if interval != 90*time.Second {
		t.Errorf("expected interval 90s, got %v", interval)
	}
	if enabled {
		t.Errorf("expected enabled to be false")
	}
}

func TestFormatBytesLarge(t *testing.T) {
	pb := uint64(1024) * 1024 * 1024 * 1024 * 1024
	if res := FormatBytes(pb); res != "1.0 PB" {
		t.Errorf("FormatBytes(1 PB) = %q, expected '1.0 PB'", res)
	}

	eb := pb * 1024
	if res := FormatBytes(eb); res != "1.0 EB" {
		t.Errorf("FormatBytes(1 EB) = %q, expected '1.0 EB'", res)
	}

	maxU := ^uint64(0)
	if res := FormatBytes(maxU); !strings.HasSuffix(res, "EB") {
		t.Errorf("FormatBytes(MaxUint64) = %q, expected EB suffix", res)
	}
}

func TestHideFileExecution(t *testing.T) {
	// Should not panic on non-existent or temporary files
	HideFile("non_existent_file_path.tmp")

	tempFile, err := os.CreateTemp("", "hide_test_*")
	if err == nil {
		tempPath := tempFile.Name()
		tempFile.Close()
		defer os.Remove(tempPath)
		HideFile(tempPath)
	}
}

func TestSingleInstanceReleaseMultiple(t *testing.T) {
	testMutex := fmt.Sprintf("Local\\DrivePulse_MultiRel_%d", time.Now().UnixNano())
	h, err := AcquireSingleInstanceNamed(testMutex)
	if err != nil {
		t.Fatalf("AcquireSingleInstanceNamed failed: %v", err)
	}

	// Multiple releases should be safe and idempotent
	h.Release()
	h.Release()
}

func TestDriveInfoAllTypes(t *testing.T) {
	types := []DriveType{
		DriveTypeFixed,
		DriveTypeRemovable,
		DriveTypeNetwork,
		DriveTypeOptical,
		DriveTypeUnknown,
		"RAM Disk",
	}

	for _, dt := range types {
		d := DriveInfo{
			Path:  "X:\\",
			Label: "",
			Type:  dt,
		}
		name := d.DisplayName()
		if !strings.Contains(name, "X:\\") || !strings.Contains(name, string(dt)) {
			t.Errorf("DisplayName() for type %s = %q", dt, name)
		}
	}
}

func TestConfigInvalidIntervalFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_interval_fallback_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")

	// Test with invalid positive interval not in AllowedIntervals
	invalidJSON := []byte(`{ "master_enabled": true, "interval_seconds": 15, "selected_drives": [] }`)
	if err := os.WriteFile(configPath, invalidJSON, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	if mgr.Get().IntervalSeconds != DefaultIntervalSeconds {
		t.Errorf("expected interval %d for unsupported value 15, got %d", DefaultIntervalSeconds, mgr.Get().IntervalSeconds)
	}

	// Test with negative interval
	invalidJSON2 := []byte(`{ "master_enabled": true, "interval_seconds": -10, "selected_drives": [] }`)
	if err := os.WriteFile(configPath, invalidJSON2, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := mgr.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if mgr.Get().IntervalSeconds != DefaultIntervalSeconds {
		t.Errorf("expected interval %d for negative value -10, got %d", DefaultIntervalSeconds, mgr.Get().IntervalSeconds)
	}
}

func TestConfigSelectedDrivesJSONMarshaling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_json_marshal_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	// Add then remove drive to make slice empty
	_ = mgr.SetDriveSelected("E:\\", true)
	_ = mgr.SetDriveSelected("E:\\", false)

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if strings.Contains(string(data), `"selected_drives": null`) {
		t.Errorf("expected selected_drives to serialize as empty array [], got null: %s", string(data))
	}
	if !strings.Contains(string(data), `"selected_drives": []`) {
		t.Errorf("expected selected_drives to contain [], got: %s", string(data))
	}
}

func TestCleanDrivesWindowsFormatting(t *testing.T) {
	input := []string{
		"e:\\",
		"E:\\",
		"e:",
		"f:/games",
		"  g:\\data  ",
		"",
		"   ",
	}

	cleaned := cleanDrives(input)

	if runtime.GOOS == "windows" {
		// Should contain uppercase E:\ and not lowercase e:\
		hasE := false
		for _, d := range cleaned {
			if d == "E:\\" {
				hasE = true
			}
			if d == "e:\\" || d == "e:" {
				t.Errorf("cleanDrives produced unnormalized Windows drive path: %s", d)
			}
		}
		if !hasE {
			t.Errorf("expected E:\\ to be present in cleaned drives: %v", cleaned)
		}
	}
}

func TestEngineSettersConcurrentWithLoop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_concurrent_engine_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 30, true)
	eng.Start()
	defer eng.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			eng.SetInterval(45 + (idx%3)*15)
			eng.SetEnabled(idx%2 == 0)
			eng.SetDrives([]string{tempDir})
			eng.TriggerPingNow()
			_, _ = eng.GetStatus()
		}(i)
	}
	wg.Wait()
}

func TestNewTrayControllerInit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_init_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	eng := NewEngine(nil, 45, true)
	ctrl := NewTrayController(mgr, eng)

	if ctrl == nil {
		t.Fatalf("expected non-nil TrayController")
	}
	if len(ctrl.slots) != maxDriveSlots {
		t.Errorf("expected %d slots, got %d", maxDriveSlots, len(ctrl.slots))
	}
}

func TestNormalizeDrivePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		res := normalizeDrivePath(tt.input)
		if res != tt.expected {
			t.Errorf("normalizeDrivePath(%q) = %q, expected %q", tt.input, res, tt.expected)
		}
	}

	if runtime.GOOS == "windows" {
		winTests := []struct {
			input    string
			expected string
		}{
			{"e:", "E:\\"},
			{"E:", "E:\\"},
			{"e:\\", "E:\\"},
			{"E:\\", "E:\\"},
			{"d:/stuff", "D:\\stuff"},
			{"  f:\\games  ", "F:\\games"},
		}
		for _, tt := range winTests {
			res := normalizeDrivePath(tt.input)
			if res != tt.expected {
				t.Errorf("normalizeDrivePath(%q) = %q, expected %q", tt.input, res, tt.expected)
			}
		}
	}
}

func TestConfigDriveSelectionSlashVariations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_slash_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	// Add with no trailing slash on Windows
	if err := mgr.SetDriveSelected("e:", true); err != nil {
		t.Fatalf("SetDriveSelected failed: %v", err)
	}

	// Should match query with trailing slash
	if !mgr.IsDriveSelected("E:\\") {
		t.Errorf("expected IsDriveSelected('E:\\') to be true")
	}
	if !mgr.IsDriveSelected("e:") {
		t.Errorf("expected IsDriveSelected('e:') to be true")
	}

	// Empty string selection should be ignored safely
	_ = mgr.SetDriveSelected("", true)
	if mgr.IsDriveSelected("") {
		t.Errorf("expected empty string to not be selected")
	}

	// Unselect using variant
	if err := mgr.SetDriveSelected("E:\\", false); err != nil {
		t.Fatalf("unselecting failed: %v", err)
	}
	if mgr.IsDriveSelected("e:") || mgr.IsDriveSelected("E:\\") {
		t.Errorf("expected drive to be unselected across all variants")
	}
}

func TestTargetInstallPathStructure(t *testing.T) {
	target, err := getTargetInstallPath()
	if err != nil {
		t.Fatalf("getTargetInstallPath failed: %v", err)
	}
	if !strings.Contains(target, "DrivePulse") && !strings.Contains(target, "drivepulse") {
		t.Errorf("unexpected target install path: %s", target)
	}
	if !strings.HasSuffix(target, TargetAppName) {
		t.Errorf("unexpected target install path suffix: %s", target)
	}
}

func TestEngineInvalidIntervalDefaults(t *testing.T) {
	eng := NewEngine(nil, -10, true)
	if eng.interval != 45*time.Second {
		t.Errorf("expected 45s default for negative interval, got %v", eng.interval)
	}

	eng.SetInterval(0)
	if eng.interval != 45*time.Second {
		t.Errorf("expected 45s default for zero interval, got %v", eng.interval)
	}
}

