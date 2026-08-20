package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

const (
	DefaultIntervalSeconds = 45
	ConfigDirName          = "DrivePulse"
	ConfigFileName         = "config.json"
	maxLogEntries          = 500
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

func GetDefaultConfigPath() (string, error) {
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

	appDir := filepath.Join(baseDir, ConfigDirName)
	return filepath.Join(appDir, ConfigFileName), nil
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

func normalizeDrivePath(path string) string {
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

func (m *ConfigManager) IsDriveSelected(drivePath string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	normPath := normalizeDrivePath(drivePath)
	if normPath == "" {
		return false
	}
	for _, d := range m.config.SelectedDrives {
		normD := normalizeDrivePath(d)
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
	normPath := normalizeDrivePath(drivePath)
	if normPath == "" {
		m.mu.Unlock()
		return nil
	}
	newDrives := make([]string, 0)

	for _, d := range m.config.SelectedDrives {
		normD := normalizeDrivePath(d)
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

// --- Rolling Session Logger ---

type LogEntry struct {
	Timestamp time.Time
	Category  string
	Message   string
}

func (e LogEntry) Formatted() string {
	return fmt.Sprintf("[%s] [%s] %s", e.Timestamp.Format("2006-01-02 15:04:05"), e.Category, e.Message)
}

type RollingLogger struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

var defaultLogger = NewRollingLogger(maxLogEntries)

func NewRollingLogger(maxSize int) *RollingLogger {
	if maxSize <= 0 {
		maxSize = maxLogEntries
	}
	return &RollingLogger{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

func (l *RollingLogger) Log(category string, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	entry := LogEntry{
		Timestamp: time.Now(),
		Category:  strings.ToUpper(strings.TrimSpace(category)),
		Message:   msg,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.entries) >= l.maxSize {
		copy(l.entries, l.entries[1:])
		l.entries[len(l.entries)-1] = entry
	} else {
		l.entries = append(l.entries, entry)
	}
}

func (l *RollingLogger) GetAll() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.entries) == 0 {
		return "No logs recorded yet.\n"
	}

	var sb strings.Builder
	sb.WriteString("=== DrivePulse Session Logs ===\n")
	for _, e := range l.entries {
		sb.WriteString(e.Formatted())
		sb.WriteByte('\n')
	}
	sb.WriteString("=== End of Logs ===\n")
	return sb.String()
}

func (l *RollingLogger) CopyToClipboard() error {
	logs := l.GetAll()
	return clipboard.WriteAll(logs)
}

func (l *RollingLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}

func (l *RollingLogger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

func logInfo(format string, args ...interface{})    { defaultLogger.Log("INFO", format, args...) }
func logPing(format string, args ...interface{})    { defaultLogger.Log("PING", format, args...) }
func logHotplug(format string, args ...interface{}) { defaultLogger.Log("HOTPLUG", format, args...) }
func logConfig(format string, args ...interface{})  { defaultLogger.Log("CONFIG", format, args...) }
func logWarn(format string, args ...interface{})    { defaultLogger.Log("WARN", format, args...) }
func logError(format string, args ...interface{})   { defaultLogger.Log("ERROR", format, args...) }
