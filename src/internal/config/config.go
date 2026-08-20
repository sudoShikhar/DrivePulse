package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/sudoShikhar/DrivePulse/src/internal/logger"
)

const (
	DefaultIntervalSeconds = 45
	ConfigDirName          = "DrivePulse"
	ConfigFileName         = "config.json"
)

var AllowedIntervals = []int{30, 45, 60, 90}

// Config represents persistent application settings.
type Config struct {
	MasterEnabled   bool     `json:"master_enabled"`
	IntervalSeconds int      `json:"interval_seconds"`
	SelectedDrives  []string `json:"selected_drives"`
	Autostart       bool     `json:"autostart"`
}

// ConfigManager coordinates configuration persistence.
type ConfigManager struct {
	mu         sync.RWMutex
	config     *Config
	configPath string
}

func DefaultConfig() *Config {
	return &Config{
		MasterEnabled:   true,
		IntervalSeconds: DefaultIntervalSeconds,
		SelectedDrives:  make([]string, 0),
		Autostart:       true,
	}
}

func GetDefaultAppDir() (string, error) {
	var baseDir string
	if runtime.GOOS == "windows" {
		baseDir = os.Getenv("APPDATA")
		if baseDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to determine user home dir: %w", err)
			}
			baseDir = filepath.Join(home, "AppData", "Roaming")
		}
	} else {
		baseDir = os.Getenv("XDG_CONFIG_HOME")
		if baseDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to determine user home dir: %w", err)
			}
			baseDir = filepath.Join(home, ".config")
		}
	}

	return filepath.Join(baseDir, ConfigDirName), nil
}

func GetDefaultConfigPath() (string, error) {
	appDir, err := GetDefaultAppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(appDir, ConfigFileName), nil
}

func GetDefaultLogsDir() (string, error) {
	appDir, err := GetDefaultAppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(appDir, logger.LogsDirName), nil
}

func NewConfigManager(customPath string) (*ConfigManager, error) {
	path := customPath
	if path == "" {
		var err error
		path, err = GetDefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	m := &ConfigManager{
		configPath: path,
		config:     DefaultConfig(),
	}

	if err := m.Load(); err != nil {
		if os.IsNotExist(err) {
			_ = m.Save()
		} else {
			return nil, err
		}
	}

	return m, nil
}

func (m *ConfigManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("invalid config json: %w", err)
	}

	validInterval := false
	for _, interval := range AllowedIntervals {
		if cfg.IntervalSeconds == interval {
			validInterval = true
			break
		}
	}
	if !validInterval {
		cfg.IntervalSeconds = DefaultIntervalSeconds
	}

	if cfg.SelectedDrives == nil {
		cfg.SelectedDrives = make([]string, 0)
	}

	m.config = &cfg
	return nil
}

func (m *ConfigManager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config json: %w", err)
	}

	tmpFile := m.configPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write tmp config file: %w", err)
	}

	if err := os.Rename(tmpFile, m.configPath); err != nil {
		_ = os.Remove(m.configPath)
		if err := os.Rename(tmpFile, m.configPath); err != nil {
			_ = os.WriteFile(m.configPath, data, 0644)
			_ = os.Remove(tmpFile)
		}
	}

	return nil
}

func (m *ConfigManager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	drives := make([]string, len(m.config.SelectedDrives))
	copy(drives, m.config.SelectedDrives)

	return Config{
		MasterEnabled:   m.config.MasterEnabled,
		IntervalSeconds: m.config.IntervalSeconds,
		SelectedDrives:  drives,
		Autostart:       m.config.Autostart,
	}
}

func (m *ConfigManager) SetMasterEnabled(enabled bool) error {
	m.mu.Lock()
	m.config.MasterEnabled = enabled
	m.mu.Unlock()
	return m.Save()
}

func (m *ConfigManager) SetInterval(seconds int) error {
	m.mu.Lock()
	m.config.IntervalSeconds = seconds
	m.mu.Unlock()
	return m.Save()
}

func (m *ConfigManager) SetAutostart(autostart bool) error {
	m.mu.Lock()
	m.config.Autostart = autostart
	m.mu.Unlock()
	return m.Save()
}

func NormalizeDrivePath(path string) string {
	d := strings.TrimSpace(path)
	if d == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		if len(d) == 2 && d[1] == ':' {
			d = strings.ToUpper(d) + `\`
		}
	}
	c := filepath.Clean(d)
	if runtime.GOOS == "windows" {
		if len(c) == 3 && c[1] == ':' && c[2] == '.' {
			c = strings.ToUpper(string(c[0])) + `:\`
		} else if len(c) >= 2 && c[1] == ':' {
			c = strings.ToUpper(string(c[0])) + c[1:]
		}
	}
	return c
}

func CleanDrives(drives []string) []string {
	var cleaned []string
	seen := make(map[string]bool)
	for _, d := range drives {
		c := NormalizeDrivePath(d)
		if c == "" {
			continue
		}
		key := c
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(c)
		}
		if !seen[key] {
			seen[key] = true
			cleaned = append(cleaned, c)
		}
	}
	return cleaned
}

func (m *ConfigManager) IsDriveSelected(drivePath string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	normPath := NormalizeDrivePath(drivePath)
	if normPath == "" {
		return false
	}
	for _, d := range m.config.SelectedDrives {
		normD := NormalizeDrivePath(d)
		if runtime.GOOS == "windows" {
			if strings.EqualFold(normD, normPath) {
				return true
			}
		} else {
			if normD == normPath {
				return true
			}
		}
	}
	return false
}

func (m *ConfigManager) SetDriveSelected(drivePath string, selected bool) error {
	m.mu.Lock()
	normPath := NormalizeDrivePath(drivePath)
	if normPath == "" {
		m.mu.Unlock()
		return nil
	}
	newDrives := make([]string, 0)

	for _, d := range m.config.SelectedDrives {
		normD := NormalizeDrivePath(d)
		match := false
		if runtime.GOOS == "windows" {
			match = strings.EqualFold(normD, normPath)
		} else {
			match = normD == normPath
		}
		if !match {
			newDrives = append(newDrives, d)
		}
	}

	if selected {
		newDrives = append(newDrives, normPath)
	}

	m.config.SelectedDrives = newDrives
	m.mu.Unlock()
	return m.Save()
}
