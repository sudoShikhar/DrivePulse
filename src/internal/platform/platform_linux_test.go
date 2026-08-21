//go:build !windows

package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudoShikhar/DrivePulse/src/internal/assets"
)

func TestIsVirtualFS(t *testing.T) {
	virtualFS := []string{
		"proc", "sysfs", "devtmpfs", "tmpfs", "cgroup", "cgroup2",
		"pstore", "securityfs", "debugfs", "tracefs", "hugetlbfs",
		"mqueue", "autofs", "squashfs", "overlay",
	}

	for _, fs := range virtualFS {
		if !isVirtualFS(fs) {
			t.Errorf("expected isVirtualFS(%q) = true", fs)
		}
	}

	physicalFS := []string{
		"ext4", "ext3", "btrfs", "xfs", "zfs", "vfat", "ntfs", "exfat", "f2fs",
	}

	for _, fs := range physicalFS {
		if isVirtualFS(fs) {
			t.Errorf("expected isVirtualFS(%q) = false", fs)
		}
	}
}

func TestGetDesktopFilePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_xdg_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)

	os.Setenv("XDG_CONFIG_HOME", tempDir)
	desktopPath, err := getDesktopFilePath()
	if err != nil {
		t.Fatalf("getDesktopFilePath failed: %v", err)
	}

	expected := filepath.Join(tempDir, "autostart", DesktopFileName)
	if desktopPath != expected {
		t.Errorf("expected %s, got %s", expected, desktopPath)
	}
}

func TestInstallLinuxIcons(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_icons_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempDir)

	if err := InstallLinuxIcons(); err != nil {
		t.Fatalf("InstallLinuxIcons failed: %v", err)
	}

	sizes := []string{"48x48", "256x256", "512x512"}
	for _, size := range sizes {
		iconPath := filepath.Join(tempDir, ".local", "share", "icons", "hicolor", size, "apps", "drivepulse.png")
		data, err := os.ReadFile(iconPath)
		if err != nil {
			t.Errorf("expected icon at %s: %v", iconPath, err)
			continue
		}
		if !bytes.Equal(data, assets.IconActivePNG) {
			t.Errorf("icon at %s content does not match IconActivePNG", iconPath)
		}
	}

	pixmapPath := filepath.Join(tempDir, ".local", "share", "pixmaps", "drivepulse.png")
	data, err := os.ReadFile(pixmapPath)
	if err != nil {
		t.Errorf("expected pixmap icon at %s: %v", pixmapPath, err)
	} else if !bytes.Equal(data, assets.IconActivePNG) {
		t.Errorf("pixmap icon content does not match IconActivePNG")
	}
}

func TestCreateMenuEntry(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_menu_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempDir)

	dummyExe := filepath.Join(tempDir, "bin", "drivepulse")
	if err := CreateMenuEntry(dummyExe); err != nil {
		t.Fatalf("CreateMenuEntry failed: %v", err)
	}

	desktopFile := filepath.Join(tempDir, ".local", "share", "applications", DesktopFileName)
	content, err := os.ReadFile(desktopFile)
	if err != nil {
		t.Fatalf("failed to read desktop menu file: %v", err)
	}

	strContent := string(content)
	if !strings.Contains(strContent, "Exec="+dummyExe) {
		t.Errorf("expected Exec=%s in desktop entry: %s", dummyExe, strContent)
	}
	if !strings.Contains(strContent, "Icon=drivepulse") {
		t.Errorf("expected Icon=drivepulse in desktop entry: %s", strContent)
	}
}

func TestSetAutostartLinux(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_autostart_linux_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)
	os.Setenv("XDG_CONFIG_HOME", tempDir)

	if err := SetAutostart(true); err != nil {
		t.Fatalf("SetAutostart(true) failed: %v", err)
	}

	enabled, err := IsAutostartEnabled()
	if err != nil {
		t.Fatalf("IsAutostartEnabled failed: %v", err)
	}
	if !enabled {
		t.Errorf("expected autostart to be enabled")
	}

	if err := SetAutostart(false); err != nil {
		t.Fatalf("SetAutostart(false) failed: %v", err)
	}

	enabled, err = IsAutostartEnabled()
	if err != nil {
		t.Fatalf("IsAutostartEnabled failed: %v", err)
	}
	if enabled {
		t.Errorf("expected autostart to be disabled")
	}
}
