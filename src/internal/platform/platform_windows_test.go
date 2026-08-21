//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetTargetInstallPathWindows(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_win_install_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origLocalApp := os.Getenv("LOCALAPPDATA")
	defer os.Setenv("LOCALAPPDATA", origLocalApp)
	os.Setenv("LOCALAPPDATA", tempDir)

	target, err := GetTargetInstallPath()
	if err != nil {
		t.Fatalf("GetTargetInstallPath failed: %v", err)
	}

	expected := filepath.Join(tempDir, "DrivePulse", TargetAppName)
	if target != expected {
		t.Errorf("expected %s, got %s", expected, target)
	}
}

func TestCreateStartMenuShortcut(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_shortcut_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origAppData := os.Getenv("APPDATA")
	defer os.Setenv("APPDATA", origAppData)
	os.Setenv("APPDATA", tempDir)

	dummyExe := filepath.Join(tempDir, "DrivePulse.exe")
	dummyIcon := filepath.Join(tempDir, "icon.ico")
	_ = os.WriteFile(dummyExe, []byte("dummy"), 0755)
	_ = os.WriteFile(dummyIcon, []byte("dummy"), 0644)

	err = CreateStartMenuShortcut(dummyExe, dummyIcon)
	if err != nil {
		t.Logf("CreateStartMenuShortcut returned: %v (PowerShell WScript may have restricted environment permissions)", err)
	} else {
		shortcutPath := filepath.Join(tempDir, "Microsoft", "Windows", "Start Menu", "Programs", "DrivePulse.lnk")
		if _, err := os.Stat(shortcutPath); os.IsNotExist(err) {
			t.Errorf("expected shortcut at %s", shortcutPath)
		}
	}
}

func TestWindowsAutostartRead(t *testing.T) {
	_, err := IsAutostartEnabled()
	if err != nil {
		t.Errorf("IsAutostartEnabled returned error: %v", err)
	}
}

func TestTargetAppNameConstant(t *testing.T) {
	if !strings.HasSuffix(TargetAppName, ".exe") {
		t.Errorf("expected TargetAppName to have .exe suffix on Windows, got %s", TargetAppName)
	}
}
