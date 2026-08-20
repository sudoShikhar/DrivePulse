package tray

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sudoShikhar/DrivePulse/src/internal/config"
	"github.com/sudoShikhar/DrivePulse/src/internal/engine"
)

func TestNewTrayControllerInit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_init_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := config.NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	eng := engine.NewEngine(nil, 45, true)
	ctrl := NewTrayController(mgr, eng)

	if ctrl == nil {
		t.Fatalf("expected non-nil TrayController")
	}
	if len(ctrl.slots) != MaxDriveSlots {
		t.Errorf("expected %d slots, got %d", MaxDriveSlots, len(ctrl.slots))
	}
	if ctrl.cfgMgr != mgr {
		t.Errorf("expected cfgMgr to be assigned")
	}
	if ctrl.engine != eng {
		t.Errorf("expected engine to be assigned")
	}
}

func TestTrayControllerStopIdempotency(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_stop_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := config.NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	eng := engine.NewEngine(nil, 45, true)
	ctrl := NewTrayController(mgr, eng)

	ctrl.stop()
	ctrl.stop() // Must not panic or deadlock

	select {
	case <-ctrl.stopChan:
		// Channel closed successfully
	default:
		t.Errorf("expected stopChan to be closed")
	}
}
