package tray

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestTrayControllerOnExit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_exit_*")
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
	eng.Start()
	ctrl := NewTrayController(mgr, eng)

	ctrl.onExit()

	select {
	case <-ctrl.stopChan:
		// Channel closed successfully
	default:
		t.Errorf("expected stopChan to be closed after onExit")
	}
}

func TestTrayControllerHandleIntervalClick(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_interval_*")
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

	ctrl.handleIntervalClick(90)

	cfg := mgr.Get()
	if cfg.IntervalSeconds != 90 {
		t.Errorf("expected interval 90 in config, got %d", cfg.IntervalSeconds)
	}
}

func TestTrayControllerHandleDriveClick(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_drive_*")
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

	// Test nil slot does not panic
	ctrl.handleDriveClick(nil)

	// Test empty path does nothing
	emptySlot := &driveSlot{drivePath: ""}
	ctrl.handleDriveClick(emptySlot)

	// Test selecting a drive
	testDrive := "D:\\"
	slot := &driveSlot{drivePath: testDrive}
	ctrl.handleDriveClick(slot)

	if !mgr.IsDriveSelected(testDrive) {
		t.Errorf("expected drive %s to be selected", testDrive)
	}

	// Test toggling off
	ctrl.handleDriveClick(slot)
	if mgr.IsDriveSelected(testDrive) {
		t.Errorf("expected drive %s to be unselected after second click", testDrive)
	}
}

func TestTrayControllerRefreshDrivesAndUI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_refresh_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	mgr, err := config.NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	// Add a non-existent drive to test offline detection path
	_ = mgr.SetDriveSelected("NON_EXISTENT_DRIVE_ROOT:\\", true)

	eng := engine.NewEngine(nil, 45, true)
	ctrl := NewTrayController(mgr, eng)

	// Refresh UI without native systray items attached
	ctrl.RefreshDrivesAndUI()

	// Disable master switch and refresh
	_ = mgr.SetMasterEnabled(false)
	ctrl.RefreshDrivesAndUI()

	// Re-enable master and clear drives
	_ = mgr.SetMasterEnabled(true)
	_ = mgr.SetDriveSelected("NON_EXISTENT_DRIVE_ROOT:\\", false)
	ctrl.RefreshDrivesAndUI()

	// Test multiple offline drives exceeding slots
	for i := 0; i < MaxDriveSlots+5; i++ {
		_ = mgr.SetDriveSelected(filepath.Join("Z:", string(rune('A'+i))+":\\"), true)
	}
	ctrl.RefreshDrivesAndUI()
}

func TestTrayControllerPeriodicPoller(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_poller_*")
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

	done := make(chan struct{})
	go func() {
		ctrl.periodicPoller()
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	ctrl.stop()

	select {
	case <-done:
		// Stopped cleanly
	case <-time.After(1 * time.Second):
		t.Fatalf("periodicPoller failed to terminate on stopChan")
	}
}

func TestTrayControllerShowTemporaryTitle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_tray_title_*")
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

	// Test nil item does not panic
	ctrl.showTemporaryTitle(nil, "temp", "reset", 10*time.Millisecond)

	// Test cancellation on stop
	ctrl.stop()
	ctrl.showTemporaryTitle(nil, "temp", "reset", 50*time.Millisecond)
}
