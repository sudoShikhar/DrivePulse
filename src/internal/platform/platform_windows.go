//go:build windows

package platform

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sudoShikhar/DrivePulse/src/internal/assets"
	"github.com/sudoShikhar/DrivePulse/src/internal/types"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	MutexName        = "Local\\DrivePulse_App_SingleInstance_Mutex"
	RegistryRunKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	RegistryValueKey = "DrivePulse"
	TargetAppName    = "DrivePulse.exe"
)

var ErrAlreadyRunning = errors.New("another instance of DrivePulse is already running")

type WindowsSingleInstanceHandle struct {
	handle windows.Handle
}

func (w *WindowsSingleInstanceHandle) Release() {
	if w.handle != 0 {
		_ = windows.CloseHandle(w.handle)
		w.handle = 0
	}
}

func AcquireSingleInstance() (types.SingleInstanceHandle, error) {
	return AcquireSingleInstanceNamed(MutexName)
}

func AcquireSingleInstanceNamed(mutexName string) (types.SingleInstanceHandle, error) {
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

func DetectDrives() ([]types.DriveInfo, error) {
	driveBitmask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("failed to get logical drives: %w", err)
	}

	var results []types.DriveInfo

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
		var driveType types.DriveType
		switch dt {
		case windows.DRIVE_REMOVABLE:
			driveType = types.DriveTypeRemovable
		case windows.DRIVE_FIXED:
			driveType = types.DriveTypeFixed
		case windows.DRIVE_REMOTE:
			driveType = types.DriveTypeNetwork
		case windows.DRIVE_CDROM:
			driveType = types.DriveTypeOptical
		case windows.DRIVE_RAMDISK:
			driveType = "RAM Disk"
		default:
			driveType = types.DriveTypeUnknown
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
			case types.DriveTypeFixed:
				volumeLabel = "Local Disk"
			case types.DriveTypeRemovable:
				volumeLabel = "USB Drive"
			case types.DriveTypeNetwork:
				volumeLabel = "Network Drive"
			case types.DriveTypeOptical:
				volumeLabel = "Optical Drive"
			default:
				volumeLabel = string(driveType)
			}
		}

		results = append(results, types.DriveInfo{
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

		installedPath, err := GetTargetInstallPath()
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

// EnsureInstalled implements zero-config auto-setup.
// If not running from %LOCALAPPDATA%\DrivePulse, it copies itself there, sets autostart,
// creates Start Menu shortcut, launches the installed copy, and returns true to signal the launcher should exit.
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

	targetDir := filepath.Dir(targetExe)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create install directory: %w", err)
	}

	srcFile, err := os.Open(currentExe)
	if err != nil {
		return false, fmt.Errorf("failed to open source executable: %w", err)
	}
	defer srcFile.Close()

	tmpExe := targetExe + ".tmp"
	dstFile, err := os.OpenFile(tmpExe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return false, fmt.Errorf("failed to create destination executable: %w", err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		_ = os.Remove(tmpExe)
		return false, fmt.Errorf("failed to copy executable: %w", err)
	}
	dstFile.Close()

	_ = os.Remove(targetExe)
	if err := os.Rename(tmpExe, targetExe); err != nil {
		_ = os.Remove(tmpExe)
		return false, fmt.Errorf("failed to replace destination executable: %w", err)
	}

	// Write icon asset to install folder for Start Menu and shell shortcut indexing
	iconPath := filepath.Join(targetDir, "icon.ico")
	_ = os.WriteFile(iconPath, assets.IconActiveICO, 0644)

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

func GetTargetInstallPath() (string, error) {
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
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return fmt.Errorf("empty folder path")
	}
	if _, err := os.Stat(cleanPath); err != nil {
		return fmt.Errorf("folder does not exist: %w", err)
	}
	cmd := exec.Command("explorer.exe", cleanPath)
	return cmd.Start()
}

func HideFile(filePath string) {
	ptr, err := syscall.UTF16PtrFromString(filePath)
	if err == nil {
		_ = windows.SetFileAttributes(ptr, windows.FILE_ATTRIBUTE_HIDDEN)
	}
}
