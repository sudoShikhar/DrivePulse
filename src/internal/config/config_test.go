package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
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

func TestNewInMemoryConfigManager(t *testing.T) {
	mgr := NewInMemoryConfigManager()
	if mgr == nil {
		t.Fatalf("expected non-nil in-memory config manager")
	}
	cfg := mgr.Get()
	if !cfg.MasterEnabled || cfg.IntervalSeconds != DefaultIntervalSeconds || !cfg.Autostart {
		t.Errorf("unexpected default config values: %+v", cfg)
	}
	if len(cfg.SelectedDrives) != 0 {
		t.Errorf("expected 0 selected drives, got %d", len(cfg.SelectedDrives))
	}
}

func TestGetDefaultLogsDir(t *testing.T) {
	logsDir, err := GetDefaultLogsDir()
	if err != nil {
		t.Fatalf("GetDefaultLogsDir failed: %v", err)
	}
	if !strings.Contains(logsDir, "DrivePulse") || !strings.HasSuffix(logsDir, "logs") {
		t.Errorf("unexpected logs dir: %s", logsDir)
	}
}

func TestConfigManagerCaseInsensitivity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific drive letter case insensitivity test on non-windows platform")
	}

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

	if !mgr.IsDriveSelected("e:\\") {
		t.Errorf("expected IsDriveSelected('e:\\') to be true")
	}

	if err := mgr.SetDriveSelected("e:\\", false); err != nil {
		t.Fatalf("SetDriveSelected(false) failed: %v", err)
	}
	if mgr.IsDriveSelected("E:\\") {
		t.Errorf("expected E:\\ to be unselected")
	}
}

func TestConfigInvalidIntervalFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_interval_fallback_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")

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

func TestNormalizeDrivePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		res := NormalizeDrivePath(tt.input)
		if res != tt.expected {
			t.Errorf("NormalizeDrivePath(%q) = %q, expected %q", tt.input, res, tt.expected)
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
			res := NormalizeDrivePath(tt.input)
			if res != tt.expected {
				t.Errorf("NormalizeDrivePath(%q) = %q, expected %q", tt.input, res, tt.expected)
			}
		}
	}
}

func TestConfigDriveSelectionSlashVariations(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific drive slash variations test on non-windows platform")
	}

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

	if err := mgr.SetDriveSelected("e:", true); err != nil {
		t.Fatalf("SetDriveSelected failed: %v", err)
	}

	if !mgr.IsDriveSelected("E:\\") {
		t.Errorf("expected IsDriveSelected('E:\\') to be true")
	}
	if !mgr.IsDriveSelected("e:") {
		t.Errorf("expected IsDriveSelected('e:') to be true")
	}

	_ = mgr.SetDriveSelected("", true)
	if mgr.IsDriveSelected("") {
		t.Errorf("expected empty string to not be selected")
	}

	if err := mgr.SetDriveSelected("E:\\", false); err != nil {
		t.Fatalf("unselecting failed: %v", err)
	}
	if mgr.IsDriveSelected("e:") || mgr.IsDriveSelected("E:\\") {
		t.Errorf("expected drive to be unselected across all variants")
	}
}

func TestCleanDrives(t *testing.T) {
	input := []string{"", "  ", "E:\\", "e:\\", "F:\\data", "C:", "C:\\"}
	cleaned := CleanDrives(input)

	if len(cleaned) == 0 {
		t.Fatalf("expected non-empty cleaned drives")
	}

	for _, d := range cleaned {
		if strings.TrimSpace(d) == "" {
			t.Errorf("expected no empty/whitespace entries in cleaned drives: %v", cleaned)
		}
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

	cleaned := CleanDrives(input)

	if runtime.GOOS == "windows" {
		hasE := false
		for _, d := range cleaned {
			if d == "E:\\" {
				hasE = true
			}
			if d == "e:\\" || d == "e:" {
				t.Errorf("CleanDrives produced unnormalized Windows drive path: %s", d)
			}
		}
		if !hasE {
			t.Errorf("expected E:\\ to be present in cleaned drives: %v", cleaned)
		}
	}
}

func TestConfigSetMasterAndAutostart(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_master_auto_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if err := mgr.SetMasterEnabled(false); err != nil {
		t.Fatalf("SetMasterEnabled failed: %v", err)
	}
	if mgr.Get().MasterEnabled {
		t.Errorf("expected MasterEnabled = false")
	}

	if err := mgr.SetAutostart(false); err != nil {
		t.Fatalf("SetAutostart failed: %v", err)
	}
	if mgr.Get().Autostart {
		t.Errorf("expected Autostart = false")
	}

	// Reload from disk to verify persistence
	mgrReloaded, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to reload manager: %v", err)
	}
	reloadedCfg := mgrReloaded.Get()
	if reloadedCfg.MasterEnabled {
		t.Errorf("expected persisted MasterEnabled = false after reload")
	}
	if reloadedCfg.Autostart {
		t.Errorf("expected persisted Autostart = false after reload")
	}
}

func TestGetDefaultAppDir(t *testing.T) {
	appDir, err := GetDefaultAppDir()
	if err != nil {
		t.Fatalf("GetDefaultAppDir failed: %v", err)
	}
	if !strings.HasSuffix(appDir, ConfigDirName) {
		t.Errorf("expected appDir to end with %q, got %q", ConfigDirName, appDir)
	}
}

func TestConfigManagerUnixPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix path test on Windows")
	}

	tempDir, err := os.MkdirTemp("", "drivepulse_unix_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	if err := mgr.SetDriveSelected("/media/usb", true); err != nil {
		t.Fatalf("SetDriveSelected failed: %v", err)
	}

	if !mgr.IsDriveSelected("/media/usb") {
		t.Errorf("expected /media/usb to be selected")
	}
	if !mgr.IsDriveSelected("/media/usb/") {
		t.Errorf("expected /media/usb/ to be normalized and selected")
	}
	if mgr.IsDriveSelected("/media/other") {
		t.Errorf("expected /media/other to not be selected")
	}

	if err := mgr.SetDriveSelected("/media/usb", false); err != nil {
		t.Fatalf("SetDriveSelected(false) failed: %v", err)
	}
	if mgr.IsDriveSelected("/media/usb") {
		t.Errorf("expected /media/usb to be unselected")
	}
}
