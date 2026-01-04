package global

import (
	"fmt"
	"image"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ConserveLee/gui-idle/internal/constants"
	"github.com/ConserveLee/gui-idle/internal/engine/screen"
	"github.com/go-vgo/robotgo"
)

// BotState defines the current phase of the automation
type BotState int

const (
	StateStopped      BotState = iota
	StateAutoDetect            // Initial state
	StateEntry                 // Scanning for entry buttons
	StateEntryWaiting          // Waiting in lobby after clicking entry (in.png visible)
	StateInGame                // In game, waiting for exit button to appear
	StateExitStep1             // Game ended, click exit button
	StateExitStep2             // Wait for out.png to appear and click (same button as step1, different visual)
	StateSearchOpen            // Step 1: Click step1/1.png to open channel list
	StateSearchSelect          // Step 2: Select Target Channel
	StateSearchVerify          // Step 3: Verify Channel Highlighted -> back to Entry
)

type Target struct {
	Name  string
	Image image.Image
}

// GlobalBot handles the specific state machine for Global Expedition
type GlobalBot struct {
	State      BotState
	AssetsDir  string

	// Assets - organized by new directory structure
	// find_game/
	targetsGames   []Target // find_game/games/*.png - game entry buttons
	targetsFinding []Target // find_game/finding.png - verify on entry screen

	// waiting/
	targetsLobby []Target // waiting/lobby.png - verify in lobby

	// in_game/
	targetsSkill []Target // in_game/skill.png - verify in game
	targetsExit  []Target // in_game/exit.png - game end exit button

	// channel/
	targetsChannelReturn []Target // channel/return.png - return button after exit
	targetsChannelOpen   []Target // channel/open.png - open channel list
	targetsChannelSelect []Target // channel/select.png - select target channel

	// Entity Tracking
	entryTracker *EntityTracker

	// Entry Waiting State
	entryWaitCount int // Count of checks in waiting state (max 10, then exit)

	// Search State Retry Counter
	searchRetryCount int // Count of failed attempts in current search state (max 5, then fallback)

	// Debug
	debugScreenshotTaken bool // Only save one debug screenshot per session

	// Dependencies
	searcher   *screen.Searcher
	logFunc    func(string)
	statusFunc func(string)
	debugFunc  func(string, ...interface{})

	// Display Offset
	displayOffsetX int
	displayOffsetY int

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
}

func NewGlobalBot(log func(string), status func(string), debug func(string, ...interface{})) *GlobalBot {
	tracker := NewEntityTracker()
	tracker.SetDebugFunc(debug)
	searcher := screen.NewSearcher()
	searcher.SetDebugFunc(debug)
	return &GlobalBot{
		State:        StateStopped,
		AssetsDir:    "assets/global_targets",
		entryTracker: tracker,
		searcher:     searcher,
		logFunc:      log,
		statusFunc:   status,
		debugFunc:    debug,
		stopChan:     make(chan struct{}),
	}
}

func (b *GlobalBot) SetDisplayID(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.searcher.SetDisplayID(id)
	
	x, y, _, _ := robotgo.GetDisplayBounds(id)
	b.displayOffsetX = x
	b.displayOffsetY = y
	b.logFunc(fmt.Sprintf("Display %d Offset set to (%d, %d)", id, x, y))
}

func (b *GlobalBot) setState(s BotState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.State = s
}

func (b *GlobalBot) Start() {
	b.mu.Lock()
	if b.State != StateStopped {
		b.mu.Unlock()
		return
	}
	
	if err := b.loadAllAssets(); err != nil {
		b.logFunc(fmt.Sprintf("Startup Error: %v", err))
		b.mu.Unlock()
		return
	}

	b.State = StateAutoDetect
	b.stopChan = make(chan struct{})
	b.mu.Unlock()

	b.logFunc("Global Expedition Bot Started. Auto-detecting state...")
	b.wg.Add(1)
	go b.loop()
}

func (b *GlobalBot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.State == StateStopped {
		return
	}

	close(b.stopChan)
	b.wg.Wait()
	b.State = StateStopped
	b.logFunc("Bot Stopped.")
	b.statusFunc("Status: Stopped")
}

func (b *GlobalBot) loop() {
	defer b.wg.Done()
	timer := time.NewTimer(0)

	for {
		select {
		case <-b.stopChan:
			timer.Stop()
			return
		case <-timer.C:
			nextInterval := b.processState()
			timer.Reset(nextInterval)
		}
	}
}

func (b *GlobalBot) processState() time.Duration {
	switch b.State {
	case StateAutoDetect:
		return b.handleAutoDetectState()
	case StateEntry:
		return b.handleEntryState()
	case StateEntryWaiting:
		return b.handleEntryWaitingState()
	case StateInGame:
		return b.handleInGameState()
	case StateExitStep1:
		return b.handleExitState()
	case StateExitStep2:
		return b.handleExitStep2State()
	case StateSearchOpen:
		return b.handleSearchOpenState()
	case StateSearchSelect:
		return b.handleSearchSelectState()
	case StateSearchVerify:
		return b.handleSearchVerifyState()
	default:
		return constants.EntryScanIntervalHighSpeed
	}
}

func (b *GlobalBot) handleAutoDetectState() time.Duration {
	b.statusFunc("Status: Auto Detecting State...")

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil {
		b.debugFunc("CaptureScreen failed: %v", err)
		return constants.EntryScanIntervalHighSpeed
	}

	check := func(targets []Target, nextState BotState, logMsg string) bool {
		for _, target := range targets {
			_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
			if found {
				b.logFunc(fmt.Sprintf("Auto-Detect: Found [%s]. State -> %s", target.Name, logMsg))
				b.searchRetryCount = 0 // Reset retry counter on state transition
				b.setState(nextState)
				return true
			}
		}
		return false
	}

	// Detection order: from "deep" states to "shallow" states
	// 1. In-game states (highest priority)
	if check(b.targetsSkill, StateInGame, "InGame(skill)") { return constants.InGameScanInterval }
	if check(b.targetsExit, StateExitStep1, "ExitStep1(exit)") { return 0 }
	if check(b.targetsLobby, StateEntryWaiting, "EntryWaiting(lobby)") { return 0 }

	// 2. Channel selection flow
	if check(b.targetsChannelReturn, StateExitStep2, "ExitStep2(return)") { return 0 }
	if check(b.targetsChannelSelect, StateSearchSelect, "SearchSelect(select)") { return 0 }
	if check(b.targetsChannelOpen, StateSearchOpen, "SearchOpen(open)") { return 0 }

	// 3. Entry screen (finding.png means we're on the entry screen)
	if check(b.targetsFinding, StateEntry, "Entry(finding)") { return 0 }
	if check(b.targetsGames, StateEntry, "Entry(games)") { return 0 }

	// Nothing found - keep scanning
	b.debugFunc("[AutoDetect] No recognizable state found")
	return constants.SearchScanInterval
}

func (b *GlobalBot) handleEntryState() time.Duration {
	b.statusFunc("Status: Scanning Entry...")

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil {
		return 400 * time.Millisecond
	}

	// Priority check: Are we already in-game? (exit button visible)
	for _, target := range b.targetsExit {
		_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.logFunc("Already in-game (exit button detected). Switching to Exit state.")
			b.entryTracker.Reset()
			b.setState(StateExitStep1)
			return 0
		}
	}

	// Secondary check: Are we in lobby? (in.png visible)
	for _, target := range b.targetsLobby {
		_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.logFunc("In lobby (in.png detected). Switching to EntryWaiting state.")
			b.entryTracker.Reset()
			b.entryWaitCount = 0
			b.setState(StateEntryWaiting)
			return 5 * time.Second
		}
	}

	// ROI Fast Path: If we have a ROI from last high priority detection,
	// first scan only that region for high priority targets
	roi := b.entryTracker.GetROI()
	if !roi.Empty() {
		// Scan ROI for highest priority templates first (sorted descending by name)
		for _, target := range b.targetsGames {
			points := b.searcher.FindAllTemplatesInROI(screenImg, target.Image, roi, constants.DefaultTolerance)
			if len(points) > 0 {
				priority := ExtractPriority(target.Name)
				templateSize := image.Point{X: target.Image.Bounds().Dx(), Y: target.Image.Bounds().Dy()}

				for _, p := range points {
					if p.Y > 950 {
						continue
					}

					entity := DetectedEntity{
						TemplateName: target.Name,
						Priority:     priority,
						Position:     p,
						TemplateSize: templateSize,
					}

					// Skip if blacklisted
					if b.entryTracker.IsBlacklisted(entity) {
						continue
					}

					// Update tracker to refresh LastSeen (prevent expiration)
					b.entryTracker.Update([]DetectedEntity{entity})

					// Found high priority entity in ROI - click immediately!
					b.debugFunc("[Entry] ROI Fast: Found %s (pri=%d) at (%d, %d)", target.Name, priority, p.X, p.Y)
					return b.clickAndVerifyEntry(screenImg, entity)
				}
			}
		}
		b.debugFunc("[Entry] ROI scan empty, falling back to full screen")
	}

	// Full Screen Scan: Collect all detected entities from all templates
	var allEntities []DetectedEntity

	for _, target := range b.targetsGames {
		points := b.searcher.FindAllTemplates(screenImg, target.Image, constants.DefaultTolerance)
		priority := ExtractPriority(target.Name)
		templateSize := image.Point{
			X: target.Image.Bounds().Dx(),
			Y: target.Image.Bounds().Dy(),
		}

		// Debug: Log raw matches count for each template
		if len(points) > 0 {
			b.debugFunc("[Entry] Template %s found %d raw matches", target.Name, len(points))
			for i, p := range points {
				b.debugFunc("[Entry]   raw[%d] at (%d, %d)", i, p.X, p.Y)
			}
		}

		for _, p := range points {
			// Y-Axis Filter: Ignore matches at the very bottom (likely false positives)
			if p.Y > 950 {
				continue
			}

			allEntities = append(allEntities, DetectedEntity{
				TemplateName: target.Name,
				Priority:     priority,
				Position:     p,
				TemplateSize: templateSize,
			})
		}
	}

	// Update tracker with all detected entities (handles TTL-based removal)
	b.entryTracker.Update(allEntities)

	if len(allEntities) == 0 {
		b.debugFunc("[Entry] No entities found on screen (templates: %d)", len(b.targetsGames))
		// Save debug screenshot once and list templates
		if !b.debugScreenshotTaken {
			b.debugScreenshotTaken = true
			// Log all loaded templates
			b.logFunc("[Debug] Loaded entry templates:")
			for _, t := range b.targetsGames {
				bounds := t.Image.Bounds()
				b.logFunc(fmt.Sprintf("  - %s: %dx%d", t.Name, bounds.Dx(), bounds.Dy()))
			}
			// Log screen dimensions
			b.logFunc(fmt.Sprintf("[Debug] Screen capture size: %dx%d", screenImg.Bounds().Dx(), screenImg.Bounds().Dy()))
			if err := b.searcher.SaveDebugScreenshot("debug_entry_screen.png"); err == nil {
				b.logFunc("[Debug] Saved screenshot to debug_entry_screen.png - compare with templates")
			}
		}
		return constants.EntryScanIntervalHighSpeed
	}

	// Filter out blacklisted entities
	validEntities := b.entryTracker.FilterBlacklisted(allEntities)
	if len(validEntities) == 0 {
		tracked, blacklisted := b.entryTracker.Stats()
		b.debugFunc("[Entry] All %d entities blacklisted (tracked=%d, blacklisted=%d)", len(allEntities), tracked, blacklisted)
		return constants.EntryScanIntervalHighSpeed
	}

	// Sort by priority (higher first) then by Y coordinate (lower on screen first)
	SortEntitiesByPriority(validEntities)

	b.debugFunc("[Entry] Detected %d entities (%d valid after blacklist filter), sorted order:",
		len(allEntities), len(validEntities))
	for i, e := range validEntities {
		clicks := b.entryTracker.GetClickCount(e)
		b.debugFunc("  [%d] %s (pri=%d) at (%d, %d) clicks=%d", i, e.TemplateName, e.Priority, e.Position.X, e.Position.Y, clicks)
	}

	// Click the highest priority entity
	entity := validEntities[0]
	return b.clickAndVerifyEntry(screenImg, entity)
}

// clickAndVerifyEntry performs click on entity and verifies success using two-step verification
func (b *GlobalBot) clickAndVerifyEntry(screenImg image.Image, entity DetectedEntity) time.Duration {
	center := entity.Center()
	clicks := b.entryTracker.GetClickCount(entity)

	b.debugFunc("[Entry] Clicking: %s at center (%d, %d) (click #%d)",
		entity.TemplateName, center.X, center.Y, clicks+1)
	b.performClick(entity.TemplateName, entity.Position.X, entity.Position.Y, entity.TemplateSize.X, entity.TemplateSize.Y)

	// Record click and update ROI for next iteration
	blacklisted := b.entryTracker.RecordClick(entity)
	b.entryTracker.SetLastHighPriority(entity) // Update ROI

	if blacklisted {
		b.logFunc(fmt.Sprintf("[Entry] Entity %s at (%d,%d) blacklisted after 7 clicks",
			entity.TemplateName, entity.Position.X, entity.Position.Y))
	}

	// Two-step verification:
	// Step 1 (Fast): Check if finding.png disappeared (left entry screen)
	// Step 2 (Slow): Check for lobby.png, skill.png, or exit.png

	time.Sleep(constants.VerifyPreWait)

	leftEntryScreen := false // Track if we actually left the entry screen

	// Try verification up to 5 times over ~1.5 seconds
	for attempt := 1; attempt <= 5; attempt++ {
		newScreenImg, err := b.searcher.CaptureScreen()
		if err != nil {
			b.debugFunc("[Entry] Verify attempt %d: CaptureScreen failed: %v", attempt, err)
			time.Sleep(constants.VerifyRetryWait)
			continue
		}

		// Fast verification: Is finding.png still visible?
		entryScreenVisible := false
		for _, target := range b.targetsFinding {
			_, _, found := b.searcher.FindTemplate(newScreenImg, target.Image, constants.DefaultTolerance)
			if found {
				entryScreenVisible = true
				break
			}
		}

		if entryScreenVisible {
			// Still on entry screen - click didn't work yet
			b.debugFunc("[Entry] Verify attempt %d: still on entry screen (finding.png visible)", attempt)
			time.Sleep(constants.VerifyRetryWait)
			continue
		}

		// Entry screen disappeared!
		leftEntryScreen = true
		b.debugFunc("[Entry] Verify attempt %d: left entry screen (finding.png gone)", attempt)

		// Check for lobby.png (waiting in lobby)
		for _, target := range b.targetsLobby {
			_, _, found := b.searcher.FindTemplate(newScreenImg, target.Image, constants.DefaultTolerance)
			if found {
				b.logFunc(fmt.Sprintf("Entered lobby [%s]. Waiting for game to start...", target.Name))
				b.entryTracker.Reset()
				b.entryWaitCount = 0
				b.setState(StateEntryWaiting)
				return 5 * time.Second
			}
		}

		// Check for skill.png (already in game)
		for _, target := range b.targetsSkill {
			_, _, found := b.searcher.FindTemplate(newScreenImg, target.Image, constants.DefaultTolerance)
			if found {
				b.logFunc(fmt.Sprintf("In game! [%s] detected. Entering InGame state...", target.Name))
				b.entryTracker.Reset()
				b.setState(StateInGame)
				return constants.InGameScanInterval
			}
		}

		// Check for exit.png (game already finished?)
		for _, target := range b.targetsExit {
			_, _, found := b.searcher.FindTemplate(newScreenImg, target.Image, constants.DefaultTolerance)
			if found {
				b.logFunc("Exit button detected. Game already finished?")
				b.entryTracker.Reset()
				b.setState(StateExitStep1)
				return 0
			}
		}

		// Left entry screen but nothing recognized yet - might be loading, try again
		b.debugFunc("[Entry] Verify attempt %d: no recognizable state, might be loading...", attempt)
		time.Sleep(constants.VerifyLoadingWait)
	}

	// Only assume InGame if we actually left the entry screen
	if leftEntryScreen {
		b.logFunc("Left entry screen, assuming InGame state...")
		b.entryTracker.Reset()
		b.setState(StateInGame)
		return constants.InGameScanInterval
	}

	// Still on entry screen after 5 attempts - click failed, continue scanning
	b.debugFunc("[Entry] Click verification failed - still on entry screen")
	return 0 // Retry immediately
}

// handleEntryWaitingState waits in lobby for game to start
// Checks every 5 seconds if lobby.png disappears (game started)
// After 10 checks (50 seconds), clicks return.png to exit and re-search
func (b *GlobalBot) handleEntryWaitingState() time.Duration {
	b.entryWaitCount++
	b.statusFunc(fmt.Sprintf("Status: Waiting in lobby... (%d/10)", b.entryWaitCount))

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil {
		return 5 * time.Second
	}

	// Check if lobby.png is still visible
	lobbyVisible := false
	for _, target := range b.targetsLobby {
		_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			lobbyVisible = true
			break
		}
	}

	if !lobbyVisible {
		// Lobby disappeared - verify with skill.png that we're in game
		for _, target := range b.targetsSkill {
			_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
			if found {
				b.logFunc(fmt.Sprintf("Game started! [%s] detected. Switching to InGame state.", target.Name))
				b.entryWaitCount = 0
				b.setState(StateInGame)
				return constants.InGameScanInterval
			}
		}
		// No skill detected but lobby gone - assume in game anyway
		b.logFunc("Lobby disappeared, switching to InGame state.")
		b.entryWaitCount = 0
		b.setState(StateInGame)
		return constants.InGameScanInterval
	}

	// Still in lobby - check if we've waited too long
	if b.entryWaitCount >= 10 {
		b.logFunc("Waited too long in lobby (50s). Exiting to re-search...")

		// Click return.png to exit lobby
		for _, target := range b.targetsChannelReturn {
			fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
			if found {
				b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
				b.logFunc(fmt.Sprintf("Clicked [%s]. Returning to channel selection.", target.Name))
				break
			}
		}

		b.entryWaitCount = 0
		b.setState(StateSearchOpen)
		return constants.SearchScanInterval
	}

	b.debugFunc("[Waiting] lobby.png still visible, wait count=%d", b.entryWaitCount)
	return 5 * time.Second // Check again in 5 seconds
}

// handleInGameState waits for the game to finish (exit button to appear)
// Scans at low frequency (30s) since games last 10-20 minutes
func (b *GlobalBot) handleInGameState() time.Duration {
	b.statusFunc("Status: In Game (waiting for exit)...")

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil {
		return constants.InGameScanInterval
	}

	// Check for exit button
	for _, target := range b.targetsExit {
		_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.logFunc("Game finished! Exit button detected.")
			b.setState(StateExitStep1)
			return 0
		}
	}

	// Still in game
	b.debugFunc("[InGame] Exit button not found, continuing to wait...")
	return constants.InGameScanInterval
}

// getTargetByName finds a target by its name
func (b *GlobalBot) getTargetByName(name string) *Target {
	for i := range b.targetsGames {
		if b.targetsGames[i].Name == name {
			return &b.targetsGames[i]
		}
	}
	return nil
}

func (b *GlobalBot) handleExitState() time.Duration {
	b.statusFunc("Status: Clicking Exit...")

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return 10 * time.Second }

	for _, target := range b.targetsExit {
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
			time.Sleep(constants.WaitAfterClickNormal)
			b.logFunc("Clicked exit. Waiting for out.png...")
			b.setState(StateExitStep2)
			return constants.WaitAfterClickNormal
		}
	}
	return 5 * time.Second
}

// handleExitStep2State waits for out.png to appear and clicks it to return to search flow
func (b *GlobalBot) handleExitStep2State() time.Duration {
	b.statusFunc("Status: Waiting for out.png...")

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return constants.SearchRetryInterval }

	for _, target := range b.targetsChannelReturn {
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
			time.Sleep(constants.WaitAfterClickNormal)
			b.logFunc("Clicked out.png. Switching to Search Flow.")
			b.setState(StateSearchOpen)
			return constants.SearchScanInterval
		}
	}

	b.debugFunc("[ExitStep2] out.png not found, waiting...")
	return constants.SearchRetryInterval
}

func (b *GlobalBot) handleSearchOpenState() time.Duration {
	b.statusFunc(fmt.Sprintf("Status: Searching [Open List]... (%d/%d)", b.searchRetryCount, constants.SearchMaxRetries))
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return constants.SearchRetryInterval }

	for _, target := range b.targetsChannelOpen {
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
			time.Sleep(constants.WaitAfterClickNormal)
			b.searchRetryCount = 0 // Reset counter on success
			b.setState(StateSearchSelect)
			return constants.WaitAfterClickNormal
		}
	}

	b.searchRetryCount++
	if b.searchRetryCount >= constants.SearchMaxRetries {
		b.logFunc("SearchOpen: Max retries reached. Falling back to AutoDetect.")
		b.searchRetryCount = 0
		b.setState(StateAutoDetect)
		return constants.SearchRetryInterval
	}
	return constants.SearchRetryInterval
}

func (b *GlobalBot) handleSearchSelectState() time.Duration {
	b.statusFunc(fmt.Sprintf("Status: Searching [Target Channel]... (%d/%d)", b.searchRetryCount, constants.SearchMaxRetries))
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return constants.SearchRetryInterval }

	for _, target := range b.targetsChannelSelect {
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
			time.Sleep(constants.WaitAfterClickNormal)
			b.searchRetryCount = 0 // Reset counter on success
			b.setState(StateSearchVerify)
			return constants.WaitAfterClickNormal
		}
	}

	b.searchRetryCount++
	if b.searchRetryCount >= constants.SearchMaxRetries {
		b.logFunc("SearchSelect: Max retries reached. Falling back to AutoDetect.")
		b.searchRetryCount = 0
		b.setState(StateAutoDetect)
		return constants.SearchRetryInterval
	}
	return constants.SearchRetryInterval
}

func (b *GlobalBot) handleSearchVerifyState() time.Duration {
	b.statusFunc(fmt.Sprintf("Status: Verifying Highlight... (%d/%d)", b.searchRetryCount, constants.SearchMaxRetries))
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return constants.SearchRetryInterval }

	for _, target := range b.targetsFinding {
		_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.logFunc(fmt.Sprintf("Verified Highlight [%s]. Cycle Complete.", target.Name))
			b.searchRetryCount = 0 // Reset counter on success
			b.entryTracker.Reset() // Reset tracker for new entry cycle
			time.Sleep(constants.WaitAfterClickNormal)
			b.setState(StateEntry)
			return 0 // Start entry scanning immediately
		}
	}

	b.searchRetryCount++
	if b.searchRetryCount >= constants.SearchMaxRetries {
		b.logFunc("SearchVerify: Max retries reached. Falling back to AutoDetect.")
		b.searchRetryCount = 0
		b.setState(StateAutoDetect)
		return constants.SearchRetryInterval
	}
	return constants.SearchRetryInterval
}

func (b *GlobalBot) performClick(name string, x, y, w, h int) {
	centerX := x + w/2
	centerY := y + h/2
	globalX := centerX + b.displayOffsetX
	globalY := centerY + b.displayOffsetY
	
	b.debugFunc(fmt.Sprintf("Clicking [%s] Center(%d, %d) [Global: %d, %d]", name, centerX, centerY, globalX, globalY))
	robotgo.MoveMouse(globalX, globalY)
	robotgo.Click("left")
}

func (b *GlobalBot) loadAllAssets() error {
	var err error

	// find_game/
	b.targetsGames, err = b.loadTargets("find_game/games")
	if err != nil { return fmt.Errorf("failed to load games: %w", err) }

	b.targetsFinding, err = b.loadSpecificTarget("find_game", "finding.png")
	if err != nil { b.debugFunc("Warning: No finding.png target found.") }

	// waiting/
	b.targetsLobby, err = b.loadSpecificTarget("waiting", "lobby.png")
	if err != nil { b.debugFunc("Warning: No lobby.png target found.") }

	// in_game/
	b.targetsSkill, err = b.loadSpecificTarget("in_game", "skill.png")
	if err != nil { b.debugFunc("Warning: No skill.png target found (needed for InGame verification).") }

	b.targetsExit, err = b.loadSpecificTarget("in_game", "exit.png")
	if err != nil { b.debugFunc("Warning: No exit.png target found.") }

	// channel/
	b.targetsChannelReturn, err = b.loadSpecificTarget("channel", "return.png")
	if err != nil { b.debugFunc("Warning: No return.png target found.") }

	b.targetsChannelOpen, err = b.loadSpecificTarget("channel", "open.png")
	if err != nil { b.debugFunc("Warning: No open.png target found.") }

	b.targetsChannelSelect, err = b.loadSpecificTarget("channel", "select.png")
	if err != nil { b.debugFunc("Warning: No select.png target found.") }

	b.logFunc(fmt.Sprintf("Loaded Assets: Games=%d, Finding=%d, Lobby=%d, Skill=%d, Exit=%d, Channel(return/open/select)=%d/%d/%d",
		len(b.targetsGames), len(b.targetsFinding), len(b.targetsLobby),
		len(b.targetsSkill), len(b.targetsExit),
		len(b.targetsChannelReturn), len(b.targetsChannelOpen), len(b.targetsChannelSelect)))
	return nil
}

// loadSpecificTarget loads a specific file from a subdirectory
func (b *GlobalBot) loadSpecificTarget(subDir, filename string) ([]Target, error) {
	path := filepath.Join(b.AssetsDir, subDir, filename)
	img, err := b.searcher.LoadImage(path)
	if err != nil {
		return nil, err
	}
	return []Target{{Name: filename, Image: img}}, nil
}

func (b *GlobalBot) loadTargets(subDir string) ([]Target, error) {
	path := filepath.Join(b.AssetsDir, subDir, "*.png")
	files, err := filepath.Glob(path)
	if err != nil { return nil, err }

	// Sort games by priority (higher number first)
	if subDir == "find_game/games" {
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
	} else {
		sort.Strings(files)
	}
	
	var targets []Target
	for _, file := range files {
		img, err := b.searcher.LoadImage(file)
		if err != nil { continue }
		name := filepath.Base(file)
		targets = append(targets, Target{Name: name, Image: img})
	}
	return targets, nil
}
