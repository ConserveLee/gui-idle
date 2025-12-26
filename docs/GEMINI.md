# Project Context: gui (Game Automation Bot)

## Project Overview
This is a modular desktop automation tool written in Go.
It uses **Fyne** for the GUI, **kbinani/screenshot** for multi-monitor screen capture, and **RobotGo** for input simulation.

**Project Type:** Golang Application (GUI)

## Directory Structure
- `main.go`: Entry point (UI Layout & Tab Controller).
- `app/`: Feature Modules (Business Logic & UI Panels).
    - `global/`: "Global Expedition" feature.
    - `normal/`: "Normal Level" feature (Placeholder).
    - `tools/`: Utility tools (Screen Capture).
- `internal/`: Private Shared Logic.
    - `engine/`: Core Automation Engine (`Bot` struct, Event Loop).
    - `engine/screen/`: Vision & Screenshot wrappers.
    - `logger/`: UI Logging system.
- `assets/`: Resources.
    - `global_targets/`: Images for Global Expedition.
    - `capture.png`: Temporary debug screenshot.

## Building and Running

### Prerequisites
- Go 1.20+
- C/C++ Compiler (Xcode Command Line Tools on macOS).

### Commands
1.  **Install Dependencies:**
    ```bash
    go mod tidy
    ```
2.  **Run:**
    ```bash
    go run .
    ```
3.  **Build:**
    ```bash
    go build -o gamebot .
    ```

## Development Conventions
- **New Features:** Add a new package in `app/`, implement a `New...Panel()` function returning `fyne.CanvasObject`, and register it in `main.go`.
- **Core Logic:** Modify `internal/engine/bot.go` for shared automation behavior.
