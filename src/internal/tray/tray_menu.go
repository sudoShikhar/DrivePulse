package tray

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/systray"
	"github.com/sudoShikhar/DrivePulse/src/internal/assets"
	"github.com/sudoShikhar/DrivePulse/src/internal/config"
	"github.com/sudoShikhar/DrivePulse/src/internal/logger"
	"github.com/sudoShikhar/DrivePulse/src/internal/platform"
	"github.com/sudoShikhar/DrivePulse/src/internal/types"
)

func (c *TrayController) onReady() {
	systray.SetIcon(assets.GetActiveIcon())
	systray.SetTitle("DrivePulse")
	systray.SetTooltip(fmt.Sprintf("DrivePulse (v%s): External Drive Keep-Alive", c.version))

	// Header
	c.headerItem = systray.AddMenuItem("DrivePulse", "Status summary")
	c.headerItem.Disable()

	// Version
	c.versionItem = systray.AddMenuItem(fmt.Sprintf("DrivePulse v%s", c.version), fmt.Sprintf("Built: %s", c.buildDate))
	c.versionItem.Disable()

	systray.AddSeparator()

	// Drive slots
	for i := 0; i < MaxDriveSlots; i++ {
		slotItem := systray.AddMenuItem("", "")
		slotItem.Hide()
		c.slots[i] = &driveSlot{item: slotItem}
	}

	systray.AddSeparator()

	// Master Toggle
	c.masterItem = systray.AddMenuItem("⚡ Master Keep-Alive: [ ON ]", "Toggle master keep-alive heartbeat")

	// Ping Now
	c.pingNowItem = systray.AddMenuItem("🔄 Ping Now", "Trigger immediate keep-alive ping")

	// Interval Submenu
	c.intervalMenu = systray.AddMenuItem("⏱️ Interval: 45s ▸", "Set keep-alive interval")
	for _, sec := range config.AllowedIntervals {
		subItem := c.intervalMenu.AddSubMenuItem(fmt.Sprintf("%d seconds", sec), fmt.Sprintf("Ping every %d seconds", sec))
		c.intervalSubs[sec] = subItem
	}

	// Copy Logs
	c.copyLogsItem = systray.AddMenuItem("📋 Copy Logs", "Copy session logs to clipboard")

	// Open Logs Folder (with memory fallback indicator)
	if logger.DefaultLogger.IsFileLoggingEnabled() {
		c.openLogsFolderItem = systray.AddMenuItem("📂 Open Logs Folder", "Open persistent 7-day logs directory")
	} else {
		c.openLogsFolderItem = systray.AddMenuItem("⚠️ Logs (Memory Only)", "File logging unavailable - logs stored in memory only")
	}

	// Autostart
	autostartLabel := "🚀 Start with Windows"
	if runtime.GOOS != "windows" {
		autostartLabel = "🚀 Start on Login"
	}
	c.autostartItem = systray.AddMenuItem(autostartLabel, "Launch automatically on system boot")

	// Refresh
	c.refreshItem = systray.AddMenuItem("🔄 Refresh Drives List", "Scan USB and storage ports")

	systray.AddSeparator()

	// Exit
	c.exitItem = systray.AddMenuItem("❌ Exit DrivePulse", "Quit DrivePulse")

	// Initial UI refresh
	c.RefreshDrivesAndUI()

	// Launch event loop
	go c.eventLoop()

	// Background poll for drive connect/disconnect every 30s
	go c.periodicPoller()
}

func (c *TrayController) RefreshDrivesAndUI() {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfg := c.cfgMgr.Get()
	detectedDrives, _ := platform.DetectDrives()

	detectedMap := make(map[string]types.DriveInfo)
	for _, d := range detectedDrives {
		detectedMap[filepath.Clean(d.Path)] = d
	}

	slotIndex := 0
	activeCount := 0
	offlineCount := 0
	var onlineActiveDrives []string

	for _, d := range detectedDrives {
		if slotIndex >= MaxDriveSlots {
			break
		}

		cleanPath := filepath.Clean(d.Path)
		isSelected := c.cfgMgr.IsDriveSelected(cleanPath)
		slot := c.slots[slotIndex]
		if slot == nil {
			slot = &driveSlot{}
			c.slots[slotIndex] = slot
		}
		slot.drivePath = cleanPath
		slot.isConfig = isSelected
		slot.isOnline = true

		prefix := "[ ] "
		suffix := " (Disabled)"
		if isSelected {
			prefix = "[✓] "
			suffix = " (Active)"
			activeCount++
			onlineActiveDrives = append(onlineActiveDrives, d.Path)
		}

		if slot.item != nil {
			title := fmt.Sprintf("%s%s - %s%s", prefix, d.Path, d.Label, suffix)
			slot.item.SetTitle(title)
			slot.item.SetTooltip(fmt.Sprintf("Type: %s | Free: %s / %s", d.Type, types.FormatBytes(d.FreeBytes), types.FormatBytes(d.TotalBytes)))
			slot.item.Show()
		}

		slotIndex++
	}

	for _, confDrive := range cfg.SelectedDrives {
		cleanPath := filepath.Clean(confDrive)
		if _, online := detectedMap[cleanPath]; !online {
			if slotIndex >= MaxDriveSlots {
				break
			}
			slot := c.slots[slotIndex]
			if slot == nil {
				slot = &driveSlot{}
				c.slots[slotIndex] = slot
			}
			slot.drivePath = cleanPath
			slot.isConfig = true
			slot.isOnline = false
			offlineCount++

			if slot.item != nil {
				title := fmt.Sprintf("[!] %s - Disconnected (Offline)", confDrive)
				slot.item.SetTitle(title)
				slot.item.SetTooltip("Configured drive is disconnected. Ping will resume when reconnected.")
				slot.item.Show()
			}

			slotIndex++
		}
	}

	for i := slotIndex; i < MaxDriveSlots; i++ {
		if c.slots[i] != nil {
			c.slots[i].drivePath = ""
			if c.slots[i].item != nil {
				c.slots[i].item.Hide()
			}
		}
	}

	c.engine.SetDrives(onlineActiveDrives)

	if c.masterItem != nil {
		if cfg.MasterEnabled {
			c.masterItem.SetTitle("⚡ Master Keep-Alive: [ ON ]")
		} else {
			c.masterItem.SetTitle("⚡ Master Keep-Alive: [ OFF ]")
		}
	}

	if c.intervalMenu != nil {
		c.intervalMenu.SetTitle(fmt.Sprintf("⏱️ Interval: %ds ▸", cfg.IntervalSeconds))
	}
	for sec, subItem := range c.intervalSubs {
		if subItem != nil {
			if sec == cfg.IntervalSeconds {
				subItem.SetTitle(fmt.Sprintf("[✓] %d seconds", sec))
			} else {
				subItem.SetTitle(fmt.Sprintf("    %d seconds", sec))
			}
		}
	}

	if c.autostartItem != nil {
		autostartName := "Start with Windows"
		if runtime.GOOS != "windows" {
			autostartName = "Start on Login"
		}
		if cfg.Autostart {
			c.autostartItem.SetTitle(fmt.Sprintf("🚀 %s [✓]", autostartName))
		} else {
			c.autostartItem.SetTitle(fmt.Sprintf("🚀 %s [ ]", autostartName))
		}
	}

	if c.openLogsFolderItem != nil {
		if logger.DefaultLogger.IsFileLoggingEnabled() {
			c.openLogsFolderItem.SetTitle("📂 Open Logs Folder")
			c.openLogsFolderItem.SetTooltip("Open persistent 7-day logs directory")
		} else {
			c.openLogsFolderItem.SetTitle("⚠️ Logs (Memory Only)")
			c.openLogsFolderItem.SetTooltip("File logging unavailable - logs stored in memory only")
		}
	}

	if c.headerItem != nil {
		if !cfg.MasterEnabled || (activeCount == 0 && offlineCount == 0) {
			systray.SetIcon(assets.GetDisabledIcon())
			if !cfg.MasterEnabled {
				c.headerItem.SetTitle("⚪ DrivePulse: Paused (Master OFF)")
				systray.SetTooltip(fmt.Sprintf("DrivePulse (v%s): Paused (Master Switch OFF)", c.version))
			} else {
				c.headerItem.SetTitle("⚪ DrivePulse: Inactive (0 drives awake)")
				systray.SetTooltip(fmt.Sprintf("DrivePulse (v%s): Inactive (0 drives selected)", c.version))
			}
		} else if offlineCount > 0 && activeCount == 0 {
			systray.SetIcon(assets.GetWarningIcon())
			c.headerItem.SetTitle(fmt.Sprintf("🟡 DrivePulse: Warning (%d drive offline)", offlineCount))
			systray.SetTooltip(fmt.Sprintf("DrivePulse (v%s): Warning (%d drive offline)", c.version, offlineCount))
		} else {
			systray.SetIcon(assets.GetActiveIcon())
			warningSuffix := ""
			if offlineCount > 0 {
				warningSuffix = fmt.Sprintf(" [%d offline]", offlineCount)
			}
			c.headerItem.SetTitle(fmt.Sprintf("🟢 DrivePulse: Active (%d awake)%s", activeCount, warningSuffix))
			systray.SetTooltip(fmt.Sprintf("DrivePulse (v%s): Active (%d drives awake)", c.version, activeCount))
		}
	}
}

func (c *TrayController) periodicPoller() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.RefreshDrivesAndUI()
		}
	}
}
