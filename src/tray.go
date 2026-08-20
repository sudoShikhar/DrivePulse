package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"
)

const maxDriveSlots = 26

type driveSlot struct {
	item      *systray.MenuItem
	drivePath string
	isConfig  bool
	isOnline  bool
}

type TrayController struct {
	mu            sync.Mutex
	cfgMgr        *ConfigManager
	engine        *Engine
	slots         []*driveSlot
	headerItem    *systray.MenuItem
	masterItem    *systray.MenuItem
	pingNowItem   *systray.MenuItem
	intervalMenu  *systray.MenuItem
	intervalSubs  map[int]*systray.MenuItem
	copyLogsItem  *systray.MenuItem
	autostartItem *systray.MenuItem
	refreshItem   *systray.MenuItem
	exitItem      *systray.MenuItem

	stopChan chan struct{}
	stopOnce sync.Once
}

func NewTrayController(cfgMgr *ConfigManager, eng *Engine) *TrayController {
	return &TrayController{
		cfgMgr:       cfgMgr,
		engine:       eng,
		slots:        make([]*driveSlot, maxDriveSlots),
		intervalSubs: make(map[int]*systray.MenuItem),
		stopChan:     make(chan struct{}),
	}
}

func (c *TrayController) Run() {
	systray.Run(c.onReady, c.onExit)
}

func (c *TrayController) onReady() {
	systray.SetIcon(getActiveIcon())
	systray.SetTitle("DrivePulse")
	systray.SetTooltip("DrivePulse: External Drive Keep-Alive")

	// Header
	c.headerItem = systray.AddMenuItem("DrivePulse", "Status summary")
	c.headerItem.Disable()

	systray.AddSeparator()

	// Drive slots
	for i := 0; i < maxDriveSlots; i++ {
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
	for _, sec := range AllowedIntervals {
		subItem := c.intervalMenu.AddSubMenuItem(fmt.Sprintf("%d seconds", sec), fmt.Sprintf("Ping every %d seconds", sec))
		c.intervalSubs[sec] = subItem
	}

	// Copy Logs
	c.copyLogsItem = systray.AddMenuItem("📋 Copy Logs", "Copy session logs to clipboard")

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
	c.refreshDrivesAndUI()

	// Launch event loop
	go c.eventLoop()

	// Background poll for drive connect/disconnect every 30s
	go c.periodicPoller()
}

func (c *TrayController) stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})
}

func (c *TrayController) onExit() {
	logInfo("DrivePulse shutting down")
	c.stop()
	c.engine.Stop()
}

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
			c.refreshDrivesAndUI()

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
			err := defaultLogger.CopyToClipboard()
			if err != nil {
				logError("Failed to copy logs to clipboard: %v", err)
			} else {
				go func() {
					c.copyLogsItem.SetTitle("✓ Logs Copied!")
					time.Sleep(2 * time.Second)
					c.copyLogsItem.SetTitle("📋 Copy Logs")
				}()
			}

		case _, ok := <-c.autostartItem.ClickedCh:
			if !ok {
				return
			}
			cfg := c.cfgMgr.Get()
			newVal := !cfg.Autostart
			_ = c.cfgMgr.SetAutostart(newVal)
			_ = SetAutostart(newVal)
			c.refreshDrivesAndUI()

		case _, ok := <-c.refreshItem.ClickedCh:
			if !ok {
				return
			}
			logInfo("Manual drive refresh triggered")
			c.refreshDrivesAndUI()

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
		logInfo("Enabled keep-alive on drive: %s", drivePath)
	} else {
		logInfo("Disabled keep-alive on drive: %s", drivePath)
	}

	c.refreshDrivesAndUI()
}

func (c *TrayController) handleIntervalClick(seconds int) {
	_ = c.cfgMgr.SetInterval(seconds)
	c.engine.SetInterval(seconds)
	c.refreshDrivesAndUI()
}

func (c *TrayController) refreshDrivesAndUI() {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfg := c.cfgMgr.Get()
	detectedDrives, _ := DetectDrives()

	detectedMap := make(map[string]DriveInfo)
	for _, d := range detectedDrives {
		detectedMap[filepath.Clean(d.Path)] = d
	}

	slotIndex := 0
	activeCount := 0
	offlineCount := 0
	var onlineActiveDrives []string

	for _, d := range detectedDrives {
		if slotIndex >= maxDriveSlots {
			break
		}

		cleanPath := filepath.Clean(d.Path)
		isSelected := c.cfgMgr.IsDriveSelected(cleanPath)
		slot := c.slots[slotIndex]
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

		title := fmt.Sprintf("%s%s - %s%s", prefix, d.Path, d.Label, suffix)
		slot.item.SetTitle(title)
		slot.item.SetTooltip(fmt.Sprintf("Type: %s | Free: %s / %s", d.Type, FormatBytes(d.FreeBytes), FormatBytes(d.TotalBytes)))
		slot.item.Show()

		slotIndex++
	}

	for _, confDrive := range cfg.SelectedDrives {
		cleanPath := filepath.Clean(confDrive)
		if _, online := detectedMap[cleanPath]; !online {
			if slotIndex >= maxDriveSlots {
				break
			}
			slot := c.slots[slotIndex]
			slot.drivePath = cleanPath
			slot.isConfig = true
			slot.isOnline = false
			offlineCount++

			title := fmt.Sprintf("[!] %s - Disconnected (Offline)", confDrive)
			slot.item.SetTitle(title)
			slot.item.SetTooltip("Configured drive is disconnected. Ping will resume when reconnected.")
			slot.item.Show()

			slotIndex++
		}
	}

	for i := slotIndex; i < maxDriveSlots; i++ {
		c.slots[i].drivePath = ""
		c.slots[i].item.Hide()
	}

	c.engine.SetDrives(onlineActiveDrives)

	if cfg.MasterEnabled {
		c.masterItem.SetTitle("⚡ Master Keep-Alive: [ ON ]")
	} else {
		c.masterItem.SetTitle("⚡ Master Keep-Alive: [ OFF ]")
	}

	c.intervalMenu.SetTitle(fmt.Sprintf("⏱️ Interval: %ds ▸", cfg.IntervalSeconds))
	for sec, subItem := range c.intervalSubs {
		if sec == cfg.IntervalSeconds {
			subItem.SetTitle(fmt.Sprintf("[✓] %d seconds", sec))
		} else {
			subItem.SetTitle(fmt.Sprintf("    %d seconds", sec))
		}
	}

	autostartName := "Start with Windows"
	if runtime.GOOS != "windows" {
		autostartName = "Start on Login"
	}
	if cfg.Autostart {
		c.autostartItem.SetTitle(fmt.Sprintf("🚀 %s [✓]", autostartName))
	} else {
		c.autostartItem.SetTitle(fmt.Sprintf("🚀 %s [ ]", autostartName))
	}

	if !cfg.MasterEnabled || (activeCount == 0 && offlineCount == 0) {
		systray.SetIcon(getDisabledIcon())
		if !cfg.MasterEnabled {
			c.headerItem.SetTitle("⚪ DrivePulse: Paused (Master OFF)")
			systray.SetTooltip("DrivePulse: Paused (Master Switch OFF)")
		} else {
			c.headerItem.SetTitle("⚪ DrivePulse: Inactive (0 drives awake)")
			systray.SetTooltip("DrivePulse: Inactive (0 drives selected)")
		}
	} else if offlineCount > 0 && activeCount == 0 {
		systray.SetIcon(getWarningIcon())
		c.headerItem.SetTitle(fmt.Sprintf("🟡 DrivePulse: Warning (%d drive offline)", offlineCount))
		systray.SetTooltip(fmt.Sprintf("DrivePulse: Warning (%d drive offline)", offlineCount))
	} else {
		systray.SetIcon(getActiveIcon())
		warningSuffix := ""
		if offlineCount > 0 {
			warningSuffix = fmt.Sprintf(" [%d offline]", offlineCount)
		}
		c.headerItem.SetTitle(fmt.Sprintf("🟢 DrivePulse: Active (%d awake)%s", activeCount, warningSuffix))
		systray.SetTooltip(fmt.Sprintf("DrivePulse: Active (%d drives awake)", activeCount))
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
			c.refreshDrivesAndUI()
		}
	}
}
