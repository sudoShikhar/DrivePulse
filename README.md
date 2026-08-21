# DrivePulse — Project Summary & Architecture Blueprint

## 1. Problem Statement & Root Cause
- **The Issue**: External 16TB mechanical hard drives (WD Elements/My Book, Seagate Expansion, etc.) have aggressive firmware-level APM / standby sleep timers (typically 30–120s of inactivity).
- **Why Explorer Freezes**: When File Explorer or any application queries the drive, the OS initiates synchronous I/O and freezes for 3–8 seconds while the mechanical platters spin up from 0 to 5400/7200 RPM.
- **Mechanical Wear**: Continual stop/start cycles wear out the spindle motor and consume head load/unload cycles.
- **The Solution**: A native Go system tray utility (**`DrivePulse`**) that sends an unbuffered timestamp heartbeat (`.drivepulse.ping` via `O_SYNC` + `fsync`) every ~45 seconds to keep selected drives responsive at all times.

---

## 2. Technology Stack & Design Decisions
- **Language**: **Go (Golang 1.26+)** for 100% native compilation and tool tracking via Go tool directives.
  - **Windows**: Compiles to `DrivePulse-windows-x64.exe` (with `-ldflags="-H=windowsgui -s -w"`, embedded `.ico` tray states, and Windows PE `.syso` resource embedding with zero CGO dependencies; auto-installs locally as `DrivePulse.exe`).
  - **Ubuntu / Linux**: Compiles to `DrivePulse-linux-x64` (native standalone ELF binary integrating with D-Bus / StatusNotifierItem (AppIndicator); auto-installs locally as `drivepulse`).
- **Footprint**: ~5–10 MB RAM, 0% CPU at idle, instant startup.
- **Asset Bundling**: Embedded directly in the binary using `//go:embed` and `go-winres`.

---

## 3. UI/UX & System Tray Dropdown Menu

### System Tray Dropdown Layout
Clicking the tray icon (Windows bottom-right taskbar / Ubuntu top bar) opens a native context dropdown:

```text
┌──────────────────────────────────────────────┐
│  🟢 DrivePulse: Active (2 drives awake)     │
├──────────────────────────────────────────────┤
│  [✓] E:\ - 16TB Elements (Active)           │  <-- Click to toggle
│  [✓] F:\ - 8TB Backup (Active)              │  <-- Click to toggle
│  [ ] D:\ - Internal HDD (Disabled)          │  <-- Click to toggle
│  [ ] C:\ - OS NVMe SSD (Disabled)           │  <-- Click to toggle
├──────────────────────────────────────────────┤
│  ⚡ Master Keep-Alive: [ ON ]                │  <-- Master pause/resume
│  🔄 Ping Now                                 │  <-- Instant heartbeat trigger
│  ⏱️ Interval: 45s ▸                          │  <-- Submenu: 30s, 45s, 60s, 90s
│  📋 Copy Logs                                │  <-- Copies session log to clipboard
│  📂 Open Logs Folder                         │  <-- Opens persistent 7-day logs folder
│  🚀 Start with Windows / Linux [✓]          │  <-- Auto-start on boot
│  🔄 Refresh Drives List                      │  <-- Re-scans USB ports
├──────────────────────────────────────────────┤
│  ❌ Exit DrivePulse                          │
└──────────────────────────────────────────────┘
```

### Visual Icon States:
- 🟢 **Vibrant Emerald Green**: Active (at least 1 drive is actively kept awake).
- ⚪ **Dark Gray Monochrome**: Inactive (all drives disabled or master switch is OFF).
- 🟡 **Amber / Warning**: Configured drive(s) temporarily disconnected.

---

## 4. State Persistence Across Restarts

1. **Storage Path**:
   - **Configuration**:
     - **Windows**: `%APPDATA%\DrivePulse\config.json`
     - **Ubuntu/Linux**: `~/.config/DrivePulse/config.json`
   - **Persistent Daily Logs (7-Day Rolling Retention)**:
     - **Windows**: `%APPDATA%\DrivePulse\logs\drivepulse-YYYY-MM-DD.log`
     - **Ubuntu/Linux**: `~/.config/DrivePulse/logs/drivepulse-YYYY-MM-DD.log`
2. **Schema (`config.json`)**:
   ```json
   {
     "master_enabled": true,
     "interval_seconds": 45,
     "selected_drives": [
       "E:\\",
       "F:\\"
     ],
     "autostart": true
   }
   ```
3. **Hotplug & Eject Resilience**:
   - On app launch, matches connected drives with `selected_drives`, checks `[✓]`, and resumes pings immediately.
   - If an enabled drive is unplugged, `DrivePulse` retains it in config and automatically resumes heartbeats the moment it is plugged back in.

---

## 5. Core Architectural Patterns

1. **Zero-Configuration Self-Install & Auto-Setup**:
   - Automatically installs executable to `%LOCALAPPDATA%\DrivePulse\DrivePulse.exe` on Windows / `~/.local/bin/drivepulse` on Linux, sets up `.desktop` launcher and login autostart.
2. **Single-Instance Protection**:
   - Windows Named Mutex / Linux `/proc` check to cleanly terminate orphan instances and guarantee only one tray icon exists.
3. **Dual In-Memory & Persistent 7-Day Rolling File Logger**:
   - Bounded 500-entry circular buffer for instant `📋 Copy Logs` clipboard access.
   - Daily rolling `.log` files (`drivepulse-YYYY-MM-DD.log`) stored in user AppData/config directory with automatic 7-day retention pruning.
   - Tray context menu includes `📂 Open Logs Folder` with graceful memory-only fallback indicator.
4. **Non-blocking UI & `forceUpdate` Channel**:
   - Context timeouts on all system operations and an asynchronous event loop with debounce protection.
5. **Automated CI/CD GitHub Actions Workflow**:
   - Cross-compiles production binaries (`DrivePulse-windows-x64.exe` and `DrivePulse-linux-x64`), attaches timestamped releases, and maintains a floating `latest` tag.

---

## 6. Command-Line Options & Flags

| Flag / Variable | Description |
| :--- | :--- |
| `-version` | Displays version and build date. |
| `-config <path>` | Specifies a custom path to `config.json`. |
| `-autostart` | Indicates launch initiated by the OS startup mechanism. |
| `-in-place` | Runs directly from the current working directory without self-installing to user AppData. |
| `DRIVEPULSE_IN_PLACE=1` | Environment variable equivalent to `-in-place`. |

---

## 7. Build Instructions

```bash
# Clean build artifacts
make clean

# Format code, organize imports, and run deep static analysis (gofmt, goimports, go vet, staticcheck)
make format

# Run all unit tests with code coverage
make test

# Run locally directly from source
make run

# Generate PE resources and cross-compile both Windows & Linux into builds/
make build
```

### Build Outputs (`builds/`)
- `builds/DrivePulse-windows-x64.exe` — Windows 64-bit GUI binary (no console window, patched with PE icon & version manifest)
- `builds/DrivePulse-linux-x64` — Linux 64-bit ELF binary (AppIndicator / tray support)
