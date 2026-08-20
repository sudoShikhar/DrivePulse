//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	MutexName        = "Local\\DrivePulse_App_SingleInstance_Mutex"
	RegistryRunKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	RegistryValueKey = "DrivePulse"
	TargetAppName    = "DrivePulse.exe"
)

type WindowsSingleInstanceHandle struct {
	handle windows.Handle
}

func (w *WindowsSingleInstanceHandle) Release() {
	if w.handle != 0 {
		_ = windows.CloseHandle(w.handle)
		w.handle = 0
	}
}

func AcquireSingleInstance() (SingleInstanceHandle, error) {
	return AcquireSingleInstanceNamed(MutexName)
}

func AcquireSingleInstanceNamed(mutexName string) (SingleInstanceHandle, error) {
	namePtr, err := syscall.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, err
	}

	h, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			if h != 0 {
				_ = windows.CloseHandle(h)
			}
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}

	return &WindowsSingleInstanceHandle{handle: h}, nil
}

func DetectDrives() ([]DriveInfo, error) {
	driveBitmask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("failed to get logical drives: %w", err)
	}

	var results []DriveInfo

	for i := 0; i < 26; i++ {
		if (driveBitmask & (1 << i)) == 0 {
			continue
		}

		driveLetter := string(rune('A' + i))
		driveRoot := driveLetter + `:\`
		driveRootPtr, err := syscall.UTF16PtrFromString(driveRoot)
		if err != nil {
			continue
		}

		dt := windows.GetDriveType(driveRootPtr)
		var driveType DriveType
		switch dt {
		case windows.DRIVE_REMOVABLE:
			driveType = DriveTypeRemovable
		case windows.DRIVE_FIXED:
			driveType = DriveTypeFixed
		case windows.DRIVE_REMOTE:
			driveType = DriveTypeNetwork
		case windows.DRIVE_CDROM:
			driveType = DriveTypeOptical
		case windows.DRIVE_RAMDISK:
			driveType = "RAM Disk"
		default:
			driveType = DriveTypeUnknown
		}

		if dt == windows.DRIVE_CDROM || dt == windows.DRIVE_NO_ROOT_DIR {
			continue
		}

		var volumeNameBuf [260]uint16
		var fsNameBuf [260]uint16
		var serialNumber uint32
		var maxComponentLength uint32
		var fsFlags uint32

		err = windows.GetVolumeInformation(
			driveRootPtr,
			&volumeNameBuf[0],
			uint32(len(volumeNameBuf)),
			&serialNumber,
			&maxComponentLength,
			&fsFlags,
			&fsNameBuf[0],
			uint32(len(fsNameBuf)),
		)

		isReady := err == nil
		volumeLabel := syscall.UTF16ToString(volumeNameBuf[:])
		fsName := syscall.UTF16ToString(fsNameBuf[:])

		var freeBytesAvailable uint64
		var totalNumberOfBytes uint64
		var totalNumberOfFreeBytes uint64

		if isReady {
			_ = windows.GetDiskFreeSpaceEx(
				driveRootPtr,
				&freeBytesAvailable,
				&totalNumberOfBytes,
				&totalNumberOfFreeBytes,
			)
		}

		if volumeLabel == "" {
			switch driveType {
			case DriveTypeFixed:
				volumeLabel = "Local Disk"
			case DriveTypeRemovable:
				volumeLabel = "USB Drive"
			case DriveTypeNetwork:
				volumeLabel = "Network Drive"
			case DriveTypeOptical:
				volumeLabel = "Optical Drive"
			default:
				volumeLabel = string(driveType)
			}
		}

		results = append(results, DriveInfo{
			Path:       strings.ToUpper(driveRoot),
			Label:      volumeLabel,
			FileSystem: fsName,
			Type:       driveType,
			TotalBytes: totalNumberOfBytes,
			FreeBytes:  freeBytesAvailable,
			IsReady:    isReady,
		})
	}

	return results, nil
}

func SetAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, RegistryRunKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer k.Close()

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}

		installedPath, err := getTargetInstallPath()
		if err == nil {
			if _, statErr := os.Stat(installedPath); statErr == nil {
				exePath = installedPath
			}
		}

		cmd := fmt.Sprintf(`"%s" -autostart`, exePath)
		return k.SetStringValue(RegistryValueKey, cmd)
	}

	err = k.DeleteValue(RegistryValueKey)
	if err != nil && !strings.Contains(err.Error(), "cannot find the file") {
		return err
	}
	return nil
}

func IsAutostartEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, RegistryRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false, nil
	}
	defer k.Close()

	val, _, err := k.GetStringValue(RegistryValueKey)
	if err != nil {
		return false, nil
	}
	return val != "", nil
}

// EnsureInstalled implements zero-config auto-setup (WarpGUI default behavior).
// If not running from %LOCALAPPDATA%\DrivePulse, it copies itself there, sets autostart,
// creates Start Menu shortcut, launches the installed copy, and returns true to signal the launcher should exit.
func EnsureInstalled() (bool, error) {
	currentExe, err := os.Executable()
	if err != nil {
		return false, err
	}

	targetExe, err := getTargetInstallPath()
	if err != nil {
		return false, err
	}
	if strings.EqualFold(filepath.Clean(currentExe), filepath.Clean(targetExe)) {
		return false, nil
	}

	targetDir := filepath.Dir(targetExe)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
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
	dstFile.Close()

	// Write icon asset to install folder for Start Menu and shell shortcut indexing
	iconPath := filepath.Join(targetDir, "icon.ico")
	_ = os.WriteFile(iconPath, iconActiveICO, 0644)

	_ = SetAutostart(true)
	_ = CreateStartMenuShortcut(targetExe, iconPath)

	// Launch installed binary with forwarded arguments
	cmd := exec.Command(targetExe, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("failed to launch installed binary: %w", err)
	}

	return true, nil
}

func CreateStartMenuShortcut(exePath string, iconPath string) error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		appData = filepath.Join(home, "AppData", "Roaming")
	}

	programsDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs")
	if err := os.MkdirAll(programsDir, 0755); err != nil {
		return err
	}

	if iconPath == "" {
		iconPath = exePath
	}

	shortcutPath := filepath.Join(programsDir, "DrivePulse.lnk")
	psScript := fmt.Sprintf(
		`$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%s'); $s.TargetPath = '%s'; $s.Description = 'DrivePulse - External Drive Keep-Alive Heartbeat'; $s.IconLocation = '%s,0'; $s.Save()`,
		strings.ReplaceAll(shortcutPath, "'", "''"),
		strings.ReplaceAll(exePath, "'", "''"),
		strings.ReplaceAll(iconPath, "'", "''"),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func getTargetInstallPath() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "DrivePulse", TargetAppName), nil
}

func OpenFolder(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty folder path")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("folder does not exist: %w", err)
	}
	cmd := exec.Command("explorer.exe", path)
	return cmd.Start()
}

func HideFile(filePath string) {
	ptr, err := syscall.UTF16PtrFromString(filePath)
	if err == nil {
		_ = windows.SetFileAttributes(ptr, windows.FILE_ATTRIBUTE_HIDDEN)
	}
}
