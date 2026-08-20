package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sudoShikhar/DrivePulse/src/internal/types"
)

func TestDetector(t *testing.T) {
	drives, err := DetectDrives()
	if err != nil {
		t.Fatalf("DetectDrives failed: %v", err)
	}

	if len(drives) == 0 {
		t.Logf("Warning: 0 drives detected on this host")
	} else {
		for _, d := range drives {
			t.Logf("Detected: %s | Label: %s | Type: %s | FS: %s | Ready: %v | Total: %s",
				d.Path, d.Label, d.Type, d.FileSystem, d.IsReady, types.FormatBytes(d.TotalBytes))
		}
	}
}

func TestSingleInstanceAcquisition(t *testing.T) {
	testMutex := fmt.Sprintf("Local\\DrivePulse_Test_Mutex_%d", time.Now().UnixNano())
	h1, err := AcquireSingleInstanceNamed(testMutex)
	if err != nil {
		t.Fatalf("first AcquireSingleInstanceNamed failed: %v", err)
	}
	defer h1.Release()

	h2, err := AcquireSingleInstanceNamed(testMutex)
	if !errors.Is(err, ErrAlreadyRunning) {
		if h2 != nil {
			h2.Release()
		}
		t.Errorf("expected ErrAlreadyRunning on second acquisition, got: %v", err)
	}
}

func TestAutostartInspection(t *testing.T) {
	_, err := IsAutostartEnabled()
	if err != nil {
		t.Logf("IsAutostartEnabled returned error on test runner: %v", err)
	}
}

func TestHideFileExecution(t *testing.T) {
	HideFile("non_existent_file_path.tmp")

	tempFile, err := os.CreateTemp("", "hide_test_*")
	if err == nil {
		tempPath := tempFile.Name()
		tempFile.Close()
		defer os.Remove(tempPath)
		HideFile(tempPath)
	}
}

func TestSingleInstanceReleaseMultiple(t *testing.T) {
	testMutex := fmt.Sprintf("Local\\DrivePulse_MultiRel_%d", time.Now().UnixNano())
	h, err := AcquireSingleInstanceNamed(testMutex)
	if err != nil {
		t.Fatalf("AcquireSingleInstanceNamed failed: %v", err)
	}

	h.Release()
	h.Release()
}

func TestTargetInstallPathStructure(t *testing.T) {
	target, err := GetTargetInstallPath()
	if err != nil {
		t.Fatalf("GetTargetInstallPath failed: %v", err)
	}
	if !strings.Contains(target, "DrivePulse") && !strings.Contains(target, "drivepulse") {
		t.Errorf("unexpected target install path: %s", target)
	}
	if !strings.HasSuffix(target, TargetAppName) {
		t.Errorf("unexpected target install path suffix: %s", target)
	}
}

func TestOpenFolderValidation(t *testing.T) {
	if err := OpenFolder(""); err == nil {
		t.Errorf("expected error for empty folder path")
	}

	if err := OpenFolder("Z:\\NonExistent_DrivePulse_Folder_12345"); err == nil {
		t.Errorf("expected error for non-existent folder")
	}
}
