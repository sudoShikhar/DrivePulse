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
}
