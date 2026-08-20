package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/sudoShikhar/DrivePulse/src/internal/config"
	"github.com/sudoShikhar/DrivePulse/src/internal/engine"
	"github.com/sudoShikhar/DrivePulse/src/internal/logger"
	"github.com/sudoShikhar/DrivePulse/src/internal/platform"
	"github.com/sudoShikhar/DrivePulse/src/internal/tray"
)

var (
	Version   = "1.0.0"
	BuildDate = "2026-08-18"
)

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
		handedOff, err := platform.EnsureInstalled()
		if err != nil {
			logger.Error("Self-install error: %v. Running in-place.", err)
		} else if handedOff {
			// Handed off execution to the installed instance; exit launcher cleanly
			return
		}
	}

	// Single instance protection
	lock, err := platform.AcquireSingleInstance()
	if err != nil {
		if errors.Is(err, platform.ErrAlreadyRunning) {
			logger.Warn("DrivePulse is already running in background. Terminating duplicate instance.")
			return
		}
		logger.Error("Failed to acquire single instance lock: %v", err)
		return
	}
	defer lock.Release()

	// Initialize persistent daily file logging (7-day retention)
	logsDir, err := config.GetDefaultLogsDir()
	if err != nil {
		logger.Warn("Unable to resolve logs directory: %v (falling back to memory-only logging)", err)
	} else if err := logger.DefaultLogger.EnableFileLogging(logsDir, logger.DefaultRetentionDays); err != nil {
		logger.Warn("Unable to enable file logging: %v (falling back to memory-only logging)", err)
	}
	defer logger.DefaultLogger.Close()

	logger.Info("Starting DrivePulse v%s (autostart=%v)", Version, *flagAutostart)

	cfgMgr, err := config.NewConfigManager(*flagConfig)
	if err != nil {
		logger.Error("Failed to load configuration: %v. Initializing with defaults.", err)
	}

	cfg := cfgMgr.Get()

	if cfg.Autostart {
		_ = platform.SetAutostart(true)
	}

	eng := engine.NewEngine(cfg.SelectedDrives, cfg.IntervalSeconds, cfg.MasterEnabled)
	eng.Start()
	defer eng.Stop()

	ctrl := tray.NewTrayController(cfgMgr, eng)
	ctrl.Run()
}
