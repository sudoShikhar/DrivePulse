package tray

import (
	"sync"

	"fyne.io/systray"
	"github.com/sudoShikhar/DrivePulse/src/internal/config"
	"github.com/sudoShikhar/DrivePulse/src/internal/engine"
	"github.com/sudoShikhar/DrivePulse/src/internal/logger"
)

const MaxDriveSlots = 26

type driveSlot struct {
	item      *systray.MenuItem
	drivePath string
	isConfig  bool
	isOnline  bool
}

type TrayController struct {
	mu                 sync.Mutex
	cfgMgr             *config.ConfigManager
	engine             *engine.Engine
	slots              []*driveSlot
	headerItem         *systray.MenuItem
	masterItem         *systray.MenuItem
	pingNowItem        *systray.MenuItem
	intervalMenu       *systray.MenuItem
	intervalSubs       map[int]*systray.MenuItem
	copyLogsItem       *systray.MenuItem
	openLogsFolderItem *systray.MenuItem
	autostartItem      *systray.MenuItem
	refreshItem        *systray.MenuItem
	exitItem           *systray.MenuItem

	stopChan chan struct{}
	stopOnce sync.Once
}

func NewTrayController(cfgMgr *config.ConfigManager, eng *engine.Engine) *TrayController {
	return &TrayController{
		cfgMgr:       cfgMgr,
		engine:       eng,
		slots:        make([]*driveSlot, MaxDriveSlots),
		intervalSubs: make(map[int]*systray.MenuItem),
		stopChan:     make(chan struct{}),
	}
}

func (c *TrayController) Run() {
	systray.Run(c.onReady, c.onExit)
}

func (c *TrayController) stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})
}

func (c *TrayController) onExit() {
	logger.Info("DrivePulse shutting down")
	c.stop()
	c.engine.Stop()
}
