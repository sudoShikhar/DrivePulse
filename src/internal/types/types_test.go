package types

import (
	"strings"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{16 * 1024 * 1024 * 1024 * 1024, "16.0 TB"},
	}

	for _, tt := range tests {
		res := FormatBytes(tt.bytes)
		if res != tt.expected {
			t.Errorf("FormatBytes(%d) = %q, expected %q", tt.bytes, res, tt.expected)
		}
	}
}

func TestDriveInfoDisplayName(t *testing.T) {
	tests := []struct {
		info     DriveInfo
		contains []string
	}{
		{
			info:     DriveInfo{Path: "E:\\", Label: "Elements", Type: DriveTypeFixed, TotalBytes: 16 * 1024 * 1024 * 1024 * 1024},
			contains: []string{"E:\\", "Elements", "16.0 TB"},
		},
		{
			info:     DriveInfo{Path: "F:\\", Label: "", Type: DriveTypeRemovable, TotalBytes: 8 * 1024 * 1024 * 1024},
			contains: []string{"F:\\", "Removable", "8.0 GB"},
		},
		{
			info:     DriveInfo{Path: "G:\\", Label: "", Type: DriveTypeFixed, TotalBytes: 0},
			contains: []string{"G:\\", "Fixed"},
		},
	}

	for _, tt := range tests {
		name := tt.info.DisplayName()
		for _, sub := range tt.contains {
			if !strings.Contains(name, sub) {
				t.Errorf("DisplayName() = %q, expected to contain %q", name, sub)
			}
		}
	}
}

func TestDriveInfoAllTypes(t *testing.T) {
	types := []DriveType{
		DriveTypeFixed,
		DriveTypeRemovable,
		DriveTypeNetwork,
		DriveTypeOptical,
		DriveTypeUnknown,
		"RAM Disk",
	}

	for _, dt := range types {
		d := DriveInfo{
			Path:  "X:\\",
			Label: "",
			Type:  dt,
		}
		name := d.DisplayName()
		if !strings.Contains(name, "X:\\") || !strings.Contains(name, string(dt)) {
			t.Errorf("DisplayName() for type %s = %q", dt, name)
		}
	}
}

func TestFormatBytesLarge(t *testing.T) {
	pb := uint64(1024) * 1024 * 1024 * 1024 * 1024
	if res := FormatBytes(pb); res != "1.0 PB" {
		t.Errorf("FormatBytes(1 PB) = %q, expected '1.0 PB'", res)
	}

	eb := pb * 1024
	if res := FormatBytes(eb); res != "1.0 EB" {
		t.Errorf("FormatBytes(1 EB) = %q, expected '1.0 EB'", res)
	}

	maxU := ^uint64(0)
	if res := FormatBytes(maxU); !strings.HasSuffix(res, "EB") {
		t.Errorf("FormatBytes(MaxUint64) = %q, expected EB suffix", res)
	}
}
