package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

var (
	Version           = "1.0.0"
	BuildDate         = "2026-08-18"
	ErrAlreadyRunning = errors.New("another instance of DrivePulse is already running")
)

type DriveType string

const (
	DriveTypeFixed     DriveType = "Fixed"
	DriveTypeRemovable DriveType = "Removable"
	DriveTypeNetwork   DriveType = "Network"
	DriveTypeOptical   DriveType = "Optical"
	DriveTypeUnknown   DriveType = "Unknown"
)

type DriveInfo struct {
	Path       string    `json:"path"`
	Label      string    `json:"label"`
	FileSystem string    `json:"file_system"`
	Type       DriveType `json:"type"`
	TotalBytes uint64    `json:"total_bytes"`
	FreeBytes  uint64    `json:"free_bytes"`
	IsReady    bool      `json:"is_ready"`
}

func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}

func (d DriveInfo) DisplayName() string {
	label := d.Label
	if label == "" {
		label = string(d.Type)
	}
	if d.TotalBytes > 0 {
		return fmt.Sprintf("%s (%s - %s)", d.Path, label, FormatBytes(d.TotalBytes))
	}
	return fmt.Sprintf("%s (%s)", d.Path, label)
}

type SingleInstanceHandle interface {
	Release()
}

func main() {
	var (
		flagVersion   = flag.Bool("version", false, "Display version and build information")
		flagConfig    = flag.String("config", "", "Custom path to config.json")
		flagAutostart = flag.Bool("autostart", false, "Indicates launch by system autostart mechanism")
		flagInPlace   = flag.Bool("in-place", false, "Run in place without self-installing to user app directory")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Printf("DrivePulse v%s (%s)\n", Version, BuildDate)
		return
	}

	if !*flagInPlace && os.Getenv("DRIVEPULSE_IN_PLACE") != "1" {
		// Zero-Configuration Self-Install & Auto-Setup (WarpGUI default behavior)
		handedOff, err := EnsureInstalled()
		if err != nil {
			logError("Self-install error: %v. Running in-place.", err)
		} else if handedOff {
			// Handed off execution to the installed instance; exit launcher cleanly
			return
		}
	}

	// Single instance protection
	lock, err := AcquireSingleInstance()
	if err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			logWarn("DrivePulse is already running in background. Terminating duplicate instance.")
			return
		}
		logError("Failed to acquire single instance lock: %v", err)
		return
	}
	defer lock.Release()

	// Initialize persistent daily file logging (7-day retention)
	logsDir, err := GetDefaultLogsDir()
	if err != nil {
		logWarn("Unable to resolve logs directory: %v (falling back to memory-only logging)", err)
	} else if err := defaultLogger.EnableFileLogging(logsDir, DefaultRetentionDays); err != nil {
		logWarn("Unable to enable file logging: %v (falling back to memory-only logging)", err)
	}
	defer defaultLogger.Close()

	logInfo("Starting DrivePulse v%s (autostart=%v)", Version, *flagAutostart)

	cfgMgr, err := NewConfigManager(*flagConfig)
	if err != nil {
		logError("Failed to load configuration: %v. Initializing with defaults.", err)
	}

	cfg := cfgMgr.Get()

	if cfg.Autostart {
		_ = SetAutostart(true)
	}

	eng := NewEngine(cfg.SelectedDrives, cfg.IntervalSeconds, cfg.MasterEnabled)
	eng.Start()
	defer eng.Stop()

	ctrl := NewTrayController(cfgMgr, eng)
	ctrl.Run()
}
