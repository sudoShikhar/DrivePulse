package engine

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPingDrive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_engine_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	res := PingDrive(tempDir)
	if !res.Success {
		t.Fatalf("PingDrive failed: %v", res.Error)
	}
	if res.Latency <= 0 {
		t.Errorf("expected positive latency, got %v", res.Latency)
	}

	pingFile := filepath.Join(tempDir, PingFileName)
	content, err := os.ReadFile(pingFile)
	if err != nil {
		t.Fatalf("failed to read ping file: %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("ping file is empty")
	}
}

func TestEngineLoop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_engine_loop_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 1, true)
	eng.Start()
	defer eng.Stop()

	time.Sleep(100 * time.Millisecond)

	status, lastPing := eng.GetStatus()
	if lastPing.IsZero() {
		t.Errorf("expected lastPing to be set")
	}
	if s, ok := status[filepath.Clean(tempDir)]; !ok || !s.Success {
		t.Errorf("expected drive status to be success, got: %+v", s)
	}

	eng.TriggerPingNow()
	time.Sleep(100 * time.Millisecond)
}

func TestEngineMultiDriveResilience(t *testing.T) {
	tempDir1, err := os.MkdirTemp("", "drivepulse_resilience_test_1_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	tempDir2, err := os.MkdirTemp("", "drivepulse_resilience_test_2_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	nonExistent := filepath.Join(tempDir1, "non_existent_drive_folder")

	eng := NewEngine([]string{tempDir1, tempDir2, nonExistent}, 1, true)
	eng.PerformPings()

	status, lastPing := eng.GetStatus()
	if lastPing.IsZero() {
		t.Errorf("expected lastPing to be recorded")
	}
	if len(status) != 3 {
		t.Errorf("expected 3 drive status results, got %d", len(status))
	}
	if s1, ok := status[filepath.Clean(tempDir1)]; !ok || !s1.Success {
		t.Errorf("expected tempDir1 to succeed, got: %+v", s1)
	}
	if s2, ok := status[filepath.Clean(tempDir2)]; !ok || !s2.Success {
		t.Errorf("expected tempDir2 to succeed, got: %+v", s2)
	}
	if s3, ok := status[filepath.Clean(nonExistent)]; !ok || s3.Success {
		t.Errorf("expected nonExistent to fail gracefully, got: %+v", s3)
	}
}

func TestEngineDynamicConfiguration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_dynamic_engine_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 30, true)
	eng.Start()
	defer eng.Stop()

	eng.SetDrives([]string{tempDir, filepath.Join(tempDir, "sub")})
	eng.SetInterval(60)
	eng.SetEnabled(false)
	eng.SetEnabled(true)
	eng.TriggerPingNow()

	time.Sleep(100 * time.Millisecond)

	status, lastPing := eng.GetStatus()
	if lastPing.IsZero() {
		t.Errorf("expected lastPing to be recorded after dynamic operations")
	}
	if len(status) == 0 {
		t.Errorf("expected drive status to be populated")
	}
}

func TestEngineStopStartIdempotency(t *testing.T) {
	eng := NewEngine(nil, 45, true)
	eng.Start()
	eng.Start()

	eng.Stop()
	eng.Stop()
}

func TestEngineDisabledState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_disabled_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 45, false)
	eng.PerformPings()

	status, lastPing := eng.GetStatus()
	if !lastPing.IsZero() {
		t.Errorf("expected lastPing to remain zero when engine is disabled")
	}
	if len(status) != 0 {
		t.Errorf("expected no status entries when engine is disabled, got %d", len(status))
	}
}

func TestPingDriveConcurrentSamePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_concurrent_ping_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := PingDrive(tempDir)
			if !res.Success {
				t.Errorf("concurrent ping failed: %v", res.Error)
			}
		}()
	}
	wg.Wait()
}

func TestPingFileContentFormat(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_ping_format_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	res := PingDrive(tempDir)
	if !res.Success {
		t.Fatalf("PingDrive failed: %v", res.Error)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, PingFileName))
	if err != nil {
		t.Fatalf("failed to read ping file: %v", err)
	}

	timestampStr := strings.TrimSpace(string(content))
	parsedTime, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		t.Fatalf("failed to parse timestamp %q as RFC3339Nano: %v", timestampStr, err)
	}

	if time.Since(parsedTime) > 5*time.Second {
		t.Errorf("parsed timestamp %v is too far from current time", parsedTime)
	}
}

func TestPingDriveEmptyAndInvalid(t *testing.T) {
	res := PingDrive("")
	if res.Success {
		t.Errorf("expected PingDrive('') to fail")
	}
	if res.Error == nil {
		t.Errorf("expected non-nil error for empty drive path")
	}

	res2 := PingDrive("Z:\\NonExistent_DrivePulse_Dir_12345")
	if res2.Success {
		t.Errorf("expected non-existent drive ping to fail")
	}
}

func TestEngineRestartability(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_restart_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 1, true)
	eng.Start()
	time.Sleep(50 * time.Millisecond)
	eng.Stop()

	eng.Start()
	time.Sleep(50 * time.Millisecond)
	eng.Stop()
}

func TestEngineSettersWhenStopped(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_stopped_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine(nil, 45, true)
	eng.SetDrives([]string{tempDir})
	eng.SetInterval(90)
	eng.SetEnabled(false)

	eng.mu.RLock()
	drives := eng.drives
	interval := eng.interval
	enabled := eng.enabled
	eng.mu.RUnlock()

	if len(drives) != 1 || drives[0] != filepath.Clean(tempDir) {
		t.Errorf("expected drives to be updated while stopped: %v", drives)
	}
	if interval != 90*time.Second {
		t.Errorf("expected interval 90s, got %v", interval)
	}
	if enabled {
		t.Errorf("expected enabled to be false")
	}
}

func TestEngineSettersConcurrentWithLoop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "drivepulse_concurrent_engine_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eng := NewEngine([]string{tempDir}, 30, true)
	eng.Start()
	defer eng.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			eng.SetInterval(45 + (idx%3)*15)
			eng.SetEnabled(idx%2 == 0)
			eng.SetDrives([]string{tempDir})
			eng.TriggerPingNow()
			_, _ = eng.GetStatus()
		}(i)
	}
	wg.Wait()
}

func TestEngineInvalidIntervalDefaults(t *testing.T) {
	eng := NewEngine(nil, -10, true)
	if eng.interval != 45*time.Second {
		t.Errorf("expected 45s default for negative interval, got %v", eng.interval)
	}

	eng.SetInterval(0)
	if eng.interval != 45*time.Second {
		t.Errorf("expected 45s default for zero interval, got %v", eng.interval)
	}
}
