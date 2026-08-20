package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sudoShikhar/DrivePulse/src/internal/config"
	"github.com/sudoShikhar/DrivePulse/src/internal/logger"
	"github.com/sudoShikhar/DrivePulse/src/internal/platform"
)

const (
	PingFileName = ".drivepulse.ping"
)

type PingStatus struct {
	DrivePath string
	Success   bool
	Latency   time.Duration
	Timestamp time.Time
	Error     error
}

type Engine struct {
	mu             sync.RWMutex
	interval       time.Duration
	drives         []string
	enabled        bool
	lastResults    map[string]PingStatus
	lastPingTime   time.Time
	triggerPing    chan struct{}
	updateDrives   chan []string
	updateInterval chan time.Duration
	toggleChan     chan bool
	stopChan       chan struct{}
	running        bool
}

func NewEngine(initialDrives []string, intervalSeconds int, enabled bool) *Engine {
	if intervalSeconds <= 0 {
		intervalSeconds = 45
	}
	return &Engine{
		interval:       time.Duration(intervalSeconds) * time.Second,
		drives:         config.CleanDrives(initialDrives),
		enabled:        enabled,
		lastResults:    make(map[string]PingStatus),
		triggerPing:    make(chan struct{}, 5),
		updateDrives:   make(chan []string, 5),
		updateInterval: make(chan time.Duration, 5),
		toggleChan:     make(chan bool, 5),
		stopChan:       make(chan struct{}),
	}
}

func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.stopChan = make(chan struct{})
	e.running = true
	e.mu.Unlock()

	go e.runLoop()
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return
	}
	e.running = false
	close(e.stopChan)
}

func (e *Engine) TriggerPingNow() {
	select {
	case e.triggerPing <- struct{}{}:
	default:
	}
}

func (e *Engine) SetDrives(drives []string) {
	cleaned := config.CleanDrives(drives)
	e.mu.Lock()
	e.drives = cleaned
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	select {
	case e.updateDrives <- cleaned:
	default:
	}
}

func (e *Engine) SetInterval(intervalSeconds int) {
	if intervalSeconds <= 0 {
		intervalSeconds = 45
	}
	dur := time.Duration(intervalSeconds) * time.Second
	e.mu.Lock()
	e.interval = dur
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	select {
	case e.updateInterval <- dur:
	default:
	}
}

func (e *Engine) SetEnabled(enabled bool) {
	e.mu.Lock()
	e.enabled = enabled
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	select {
	case e.toggleChan <- enabled:
	default:
	}
}

func (e *Engine) GetStatus() (map[string]PingStatus, time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	copied := make(map[string]PingStatus, len(e.lastResults))
	for k, v := range e.lastResults {
		copied[k] = v
	}
	return copied, e.lastPingTime
}

func PingDrive(drivePath string) PingStatus {
	start := time.Now()
	drivePath = strings.TrimSpace(drivePath)

	status := PingStatus{
		DrivePath: drivePath,
		Timestamp: start,
	}

	if drivePath == "" {
		status.Success = false
		status.Error = errors.New("empty drive path")
		return status
	}

	pingPath := filepath.Join(drivePath, PingFileName)

	file, err := os.OpenFile(pingPath, os.O_CREATE|os.O_WRONLY|os.O_SYNC|os.O_TRUNC, 0666)
	if err != nil {
		status.Success = false
		status.Error = err
		status.Latency = time.Since(start)
		return status
	}

	timestampData := []byte(fmt.Sprintf("%s\n", time.Now().UTC().Format(time.RFC3339Nano)))
	_, writeErr := file.Write(timestampData)
	syncErr := file.Sync()
	closeErr := file.Close()

	platform.HideFile(pingPath)

	latency := time.Since(start)
	status.Latency = latency

	if writeErr != nil {
		status.Success = false
		status.Error = writeErr
	} else if syncErr != nil {
		status.Success = false
		status.Error = syncErr
	} else if closeErr != nil {
		status.Success = false
		status.Error = closeErr
	} else {
		status.Success = true
	}

	return status
}

func (e *Engine) runLoop() {
	e.mu.RLock()
	curInterval := e.interval
	e.mu.RUnlock()

	ticker := time.NewTicker(curInterval)
	defer ticker.Stop()

	e.PerformPings()

	for {
		select {
		case <-e.stopChan:
			return

		case <-ticker.C:
			e.PerformPings()

		case <-e.triggerPing:
			logger.Info("Manual keep-alive trigger initiated")
			e.PerformPings()

		case newDrives := <-e.updateDrives:
			e.mu.Lock()
			e.drives = newDrives
			e.mu.Unlock()
			logger.Config("Target drives updated: %v", newDrives)
			e.PerformPings()

		case newInterval := <-e.updateInterval:
			e.mu.Lock()
			e.interval = newInterval
			e.mu.Unlock()
			ticker.Reset(newInterval)
			logger.Config("Heartbeat interval updated to %v", newInterval)

		case enabled := <-e.toggleChan:
			e.mu.Lock()
			e.enabled = enabled
			e.mu.Unlock()
			if enabled {
				logger.Info("Master Keep-Alive resumed [ON]")
				e.PerformPings()
			} else {
				logger.Info("Master Keep-Alive paused [OFF]")
			}
		}
	}
}

func (e *Engine) PerformPings() {
	e.mu.RLock()
	enabled := e.enabled
	drives := make([]string, len(e.drives))
	copy(drives, e.drives)
	e.mu.RUnlock()

	if !enabled || len(drives) == 0 {
		return
	}

	var resultsMu sync.Mutex
	results := make([]PingStatus, 0, len(drives))
	var wg sync.WaitGroup

	for _, drive := range drives {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			res := PingDrive(d)

			resultsMu.Lock()
			results = append(results, res)
			resultsMu.Unlock()

			if res.Success {
				logger.Ping("Drive %s -> OK (latency: %v)", res.DrivePath, res.Latency.Round(time.Millisecond))
			} else {
				logger.Error("Drive %s -> FAILED: %v", res.DrivePath, res.Error)
			}
		}(drive)
	}

	wg.Wait()

	now := time.Now()
	e.mu.Lock()
	e.lastPingTime = now
	for _, res := range results {
		e.lastResults[filepath.Clean(res.DrivePath)] = res
	}
	e.mu.Unlock()
}
