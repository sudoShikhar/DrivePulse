package types

import (
	"fmt"
)

type DriveType string

const (
	DriveTypeFixed     DriveType = "Fixed"
	DriveTypeRemovable DriveType = "Removable"
	DriveTypeNetwork   DriveType = "Network"
	DriveTypeOptical   DriveType = "Optical"
	DriveTypeUnknown   DriveType = "Unknown"
)

type DriveInfo struct {
	Path       string    `json:"path"`
	Label      string    `json:"label"`
	FileSystem string    `json:"file_system"`
	Type       DriveType `json:"type"`
	TotalBytes uint64    `json:"total_bytes"`
	FreeBytes  uint64    `json:"free_bytes"`
	IsReady    bool      `json:"is_ready"`
}

func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}

func (d DriveInfo) DisplayName() string {
	label := d.Label
	if label == "" {
		label = string(d.Type)
	}
	if d.TotalBytes > 0 {
		return fmt.Sprintf("%s (%s - %s)", d.Path, label, FormatBytes(d.TotalBytes))
	}
	return fmt.Sprintf("%s (%s)", d.Path, label)
}

type SingleInstanceHandle interface {
	Release()
}
