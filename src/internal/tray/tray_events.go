package tray

import (
	"time"

	"fyne.io/systray"
	"github.com/sudoShikhar/DrivePulse/src/internal/logger"
	"github.com/sudoShikhar/DrivePulse/src/internal/platform"
)

func (c *TrayController) eventLoop() {
	for i := range c.slots {
		slot := c.slots[i]
		go func(s *driveSlot) {
			for {
				select {
				case <-c.stopChan:
					return
				case _, ok := <-s.item.ClickedCh:
					if !ok {
						return
					}
					c.handleDriveClick(s)
				}
			}
		}(slot)
	}

	for sec, subItem := range c.intervalSubs {
		go func(seconds int, item *systray.MenuItem) {
			for {
				select {
				case <-c.stopChan:
					return
				case _, ok := <-item.ClickedCh:
					if !ok {
						return
					}
					c.handleIntervalClick(seconds)
				}
			}
		}(sec, subItem)
	}

	for {
		select {
		case <-c.stopChan:
			return

		case _, ok := <-c.masterItem.ClickedCh:
			if !ok {
				return
			}
			cfg := c.cfgMgr.Get()
			newVal := !cfg.MasterEnabled
			_ = c.cfgMgr.SetMasterEnabled(newVal)
			c.engine.SetEnabled(newVal)
			c.RefreshDrivesAndUI()

		case _, ok := <-c.pingNowItem.ClickedCh:
			if !ok {
				return
			}
			c.engine.TriggerPingNow()
			go func() {
				c.pingNowItem.SetTitle("⚡ Pinging...")
				time.Sleep(1 * time.Second)
				c.pingNowItem.SetTitle("🔄 Ping Now")
			}()

		case _, ok := <-c.copyLogsItem.ClickedCh:
			if !ok {
				return
			}
			err := logger.DefaultLogger.CopyToClipboard()
			if err != nil {
				logger.Error("Failed to copy logs to clipboard: %v", err)
			} else {
				go func() {
					c.copyLogsItem.SetTitle("✓ Logs Copied!")
					time.Sleep(2 * time.Second)
					c.copyLogsItem.SetTitle("📋 Copy Logs")
				}()
			}

		case _, ok := <-c.openLogsFolderItem.ClickedCh:
			if !ok {
				return
			}
			if logger.DefaultLogger.IsFileLoggingEnabled() {
				logsDir := logger.DefaultLogger.GetLogsDir()
				if err := platform.OpenFolder(logsDir); err != nil {
					logger.Error("Failed to open logs folder: %v", err)
					go func() {
						c.openLogsFolderItem.SetTitle("⚠️ Error Opening Folder")
						time.Sleep(2 * time.Second)
						c.openLogsFolderItem.SetTitle("📂 Open Logs Folder")
					}()
				}
			} else {
				_ = logger.DefaultLogger.CopyToClipboard()
				go func() {
					c.openLogsFolderItem.SetTitle("📋 Copied (Memory Only)")
					time.Sleep(2 * time.Second)
					c.openLogsFolderItem.SetTitle("⚠️ Logs (Memory Only)")
				}()
			}

		case _, ok := <-c.autostartItem.ClickedCh:
			if !ok {
				return
			}
			cfg := c.cfgMgr.Get()
			newVal := !cfg.Autostart
			_ = c.cfgMgr.SetAutostart(newVal)
			_ = platform.SetAutostart(newVal)
			c.RefreshDrivesAndUI()

		case _, ok := <-c.refreshItem.ClickedCh:
			if !ok {
				return
			}
			logger.Info("Manual drive refresh triggered")
			c.RefreshDrivesAndUI()

		case _, ok := <-c.exitItem.ClickedCh:
			if !ok {
				return
			}
			c.stop()
			systray.Quit()
			return
		}
	}
}

func (c *TrayController) handleDriveClick(s *driveSlot) {
	if s == nil {
		return
	}

	c.mu.Lock()
	drivePath := s.drivePath
	c.mu.Unlock()

	if drivePath == "" {
		return
	}

	isSelected := c.cfgMgr.IsDriveSelected(drivePath)
	newSelected := !isSelected
	_ = c.cfgMgr.SetDriveSelected(drivePath, newSelected)

	cfg := c.cfgMgr.Get()
	c.engine.SetDrives(cfg.SelectedDrives)

	if newSelected {
		logger.Info("Enabled keep-alive on drive: %s", drivePath)
	} else {
		logger.Info("Disabled keep-alive on drive: %s", drivePath)
	}

	c.RefreshDrivesAndUI()
}

func (c *TrayController) handleIntervalClick(seconds int) {
	_ = c.cfgMgr.SetInterval(seconds)
	c.engine.SetInterval(seconds)
	c.RefreshDrivesAndUI()
}
