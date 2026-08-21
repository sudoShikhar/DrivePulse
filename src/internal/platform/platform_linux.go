//go:build !windows

package platform

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sudoShikhar/DrivePulse/src/internal/assets"
	"github.com/sudoShikhar/DrivePulse/src/internal/types"
	"golang.org/x/sys/unix"
)

const (
	DesktopFileName = "drivepulse.desktop"
	TargetAppName   = "drivepulse"
)

var ErrAlreadyRunning = errors.New("another instance of DrivePulse is already running")

type LinuxSingleInstanceHandle struct {
	file *os.File
}

func (l *LinuxSingleInstanceHandle) Release() {
	if l.file != nil {
		_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
		_ = l.file.Close()
		l.file = nil
	}
}

func AcquireSingleInstance() (types.SingleInstanceHandle, error) {
	return AcquireSingleInstanceNamed("drivepulse.lock")
}

func AcquireSingleInstanceNamed(lockName string) (types.SingleInstanceHandle, error) {
	lockPath := filepath.Join(os.TempDir(), lockName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		f.Close()
		return nil, ErrAlreadyRunning
	}

	return &LinuxSingleInstanceHandle{file: f}, nil
}

func DetectDrives() ([]types.DriveInfo, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return []types.DriveInfo{
			{Path: "/", Label: "Root", FileSystem: "rootfs", Type: types.DriveTypeFixed, IsReady: true},
		}, nil
	}
	defer file.Close()

	var results []types.DriveInfo
	seenMounts := make(map[string]bool)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		if !strings.HasPrefix(device, "/dev/") || isVirtualFS(fsType) || seenMounts[mountPoint] {
			continue
		}
		seenMounts[mountPoint] = true

		var stat unix.Statfs_t
		if err := unix.Statfs(mountPoint, &stat); err != nil {
			continue
		}

		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bavail * uint64(stat.Bsize)

		driveType := types.DriveTypeFixed
		if strings.HasPrefix(mountPoint, "/media/") || strings.HasPrefix(mountPoint, "/run/media/") {
			driveType = types.DriveTypeRemovable
		}

		label := filepath.Base(mountPoint)
		if mountPoint == "/" {
			label = "Root OS"
		}

		results = append(results, types.DriveInfo{
			Path:       mountPoint,
			Label:      label,
			FileSystem: fsType,
			Type:       driveType,
			TotalBytes: totalBytes,
			FreeBytes:  freeBytes,
			IsReady:    true,
		})
	}

	return results, nil
}

func isVirtualFS(fsType string) bool {
	switch fsType {
	case "proc", "sysfs", "devtmpfs", "tmpfs", "cgroup", "cgroup2", "pstore",
		"securityfs", "debugfs", "tracefs", "hugetlbfs", "mqueue", "autofs",
		"squashfs", "overlay":
		return true
	default:
		return false
	}
}

func getDesktopFilePath() (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "autostart", DesktopFileName), nil
}

func GetTargetInstallPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", TargetAppName), nil
}

func IsAutostartEnabled() (bool, error) {
	path, err := getDesktopFilePath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	return err == nil, nil
}

func SetAutostart(enable bool) error {
	desktopPath, err := getDesktopFilePath()
	if err != nil {
		return err
	}

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}

		targetExe, err := GetTargetInstallPath()
		if err == nil {
			if _, statErr := os.Stat(targetExe); statErr == nil {
				exePath = targetExe
			}
		}

		if err := os.MkdirAll(filepath.Dir(desktopPath), 0755); err != nil {
			return err
		}

		content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=DrivePulse
Comment=External Drive Keep-Alive Heartbeat Utility
Exec=%s -autostart
Icon=drivepulse
Terminal=false
Categories=Utility;
X-GNOME-Autostart-enabled=true
`, exePath)

		return os.WriteFile(desktopPath, []byte(content), 0644)
	}

	if err := os.Remove(desktopPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// EnsureInstalled implements zero-config auto-setup.
// If not running from ~/.local/bin/drivepulse, it copies itself there, sets up autostart and app menu entry,
// launches the installed copy, and returns true to signal the launcher should exit.
func EnsureInstalled() (bool, error) {
	currentExe, err := os.Executable()
	if err != nil {
		return false, err
	}

	targetExe, err := GetTargetInstallPath()
	if err != nil {
		return false, err
	}

	if strings.EqualFold(filepath.Clean(currentExe), filepath.Clean(targetExe)) {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(targetExe), 0755); err != nil {
		return false, fmt.Errorf("failed to create install directory: %w", err)
	}

	srcFile, err := os.Open(currentExe)
	if err != nil {
		return false, fmt.Errorf("failed to open source executable: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(targetExe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return false, fmt.Errorf("failed to create destination executable: %w", err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		return false, fmt.Errorf("failed to copy executable: %w", err)
	}
	_ = dstFile.Close()

	_ = InstallLinuxIcons()
	_ = SetAutostart(true)
	_ = CreateMenuEntry(targetExe)

	cmd := exec.Command(targetExe, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("failed to launch installed binary: %w", err)
	}

	return true, nil
}

func InstallLinuxIcons() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Install into standard XDG hicolor icon directories
	sizes := []string{"48x48", "256x256", "512x512"}
	for _, size := range sizes {
		iconDir := filepath.Join(home, ".local", "share", "icons", "hicolor", size, "apps")
		if err := os.MkdirAll(iconDir, 0755); err != nil {
			continue
		}
		iconPath := filepath.Join(iconDir, "drivepulse.png")
		_ = os.WriteFile(iconPath, assets.IconActivePNG, 0644)
	}

	// Also install to pixmaps for legacy desktop search
	pixmapsDir := filepath.Join(home, ".local", "share", "pixmaps")
	if err := os.MkdirAll(pixmapsDir, 0755); err == nil {
		_ = os.WriteFile(filepath.Join(pixmapsDir, "drivepulse.png"), assets.IconActivePNG, 0644)
	}

	return nil
}

func CreateMenuEntry(exePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	appsDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		return err
	}

	desktopPath := filepath.Join(appsDir, DesktopFileName)
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=DrivePulse
Comment=External Drive Keep-Alive Heartbeat Utility
Exec=%s
Icon=drivepulse
Terminal=false
Categories=Utility;System;
`, exePath)

	return os.WriteFile(desktopPath, []byte(content), 0644)
}

func HideFile(filePath string) {
	// No-op for Unix systems
}

func OpenFolder(path string) error {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return fmt.Errorf("empty folder path")
	}
	if _, err := os.Stat(cleanPath); err != nil {
		return fmt.Errorf("folder does not exist: %w", err)
	}
	cmd := exec.Command("xdg-open", cleanPath)
	return cmd.Start()
}
