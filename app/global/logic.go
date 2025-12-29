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
	StateExitStep1             // Waiting for game to finish
	StateSearchOpen            // Step 1: Open Channel List
	StateSearchSelect          // Step 2: Select Target Channel
	StateSearchVerify          // Step 3: Verify Channel Highlighted
)

type Target struct {
	Name  string
	Image image.Image
}

// GlobalBot handles the specific state machine for Global Expedition
type GlobalBot struct {
	State      BotState
	AssetsDir  string

	// Assets
	targetsEntry        []Target
	targetsEntryVerify  []Target // entry/verify/in.png for detecting lobby
	targetsEntryOut     []Target // entry/verify/out.png for exiting lobby
	targetsExit         []Target
	targetsSearchStep1  []Target
	targetsSearchStep2  []Target
	targetsSearchVerify []Target

	// Entity Tracking
	entryTracker *EntityTracker

	// Entry Waiting State
	entryWaitCount int // Count of checks in waiting state (max 10, then exit)

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
	return &GlobalBot{
		State:        StateStopped,
		AssetsDir:    "assets/global_targets",
		entryTracker: NewEntityTracker(),
		searcher:     screen.NewSearcher(),
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
	case StateExitStep1:
		return b.handleExitState()
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
				b.setState(nextState)
				return true
			}
		}
		return false
	}

	if check(b.targetsExit, StateExitStep1, "Exit") { return 0 }
	if check(b.targetsEntryVerify, StateEntryWaiting, "EntryWaiting(Lobby)") { return 0 }
	if check(b.targetsEntry, StateEntry, "Entry") { return 0 }
	if check(b.targetsSearchStep1, StateSearchOpen, "Search(Open)") { return 0 }
	if check(b.targetsSearchStep2, StateSearchSelect, "Search(Select)") { return 0 }
	if check(b.targetsSearchVerify, StateSearchVerify, "Search(Verify)") { return 0 }

	return constants.SearchScanInterval
}

func (b *GlobalBot) handleEntryState() time.Duration {
	b.statusFunc("Status: Scanning Entry...")

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil {
		return 400 * time.Millisecond
	}

	// ROI Fast Path: If we have a ROI from last high priority detection,
	// first scan only that region for high priority targets
	roi := b.entryTracker.GetROI()
	if !roi.Empty() {
		// Scan ROI for highest priority templates first (sorted descending by name)
		for _, target := range b.targetsEntry {
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

	for _, target := range b.targetsEntry {
		points := b.searcher.FindAllTemplates(screenImg, target.Image, constants.DefaultTolerance)
		priority := ExtractPriority(target.Name)
		templateSize := image.Point{
			X: target.Image.Bounds().Dx(),
			Y: target.Image.Bounds().Dy(),
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

// clickAndVerifyEntry performs click on entity and verifies success
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

	time.Sleep(100 * time.Millisecond)

	// Verify: check if entry/verify/in.png appears (indicates we entered the lobby)
	// in.png = lobby waiting screen, need to wait for game to actually start
	newScreenImg, err := b.searcher.CaptureScreen()
	if err == nil && len(b.targetsEntryVerify) > 0 {
		for _, verifyTarget := range b.targetsEntryVerify {
			_, _, found := b.searcher.FindTemplate(newScreenImg, verifyTarget.Image, constants.DefaultTolerance)
			if found {
				b.logFunc(fmt.Sprintf("Entered lobby [%s]. Waiting for game to start...", verifyTarget.Name))
				b.entryTracker.Reset() // Reset tracker for next cycle
				b.entryWaitCount = 0   // Reset wait counter
				b.setState(StateEntryWaiting)
				return 5 * time.Second // First check after 5 seconds
			}
		}
	}

	// Not verified - continue scanning (entity might have been grabbed by someone else)
	return 0 // Retry immediately
}

// handleEntryWaitingState waits in lobby for game to start
// Checks every 5 seconds if in.png disappears (game started)
// After 10 checks (50 seconds), clicks out.png to exit and re-search
func (b *GlobalBot) handleEntryWaitingState() time.Duration {
	b.entryWaitCount++
	b.statusFunc(fmt.Sprintf("Status: Waiting in lobby... (%d/10)", b.entryWaitCount))

	screenImg, err := b.searcher.CaptureScreen()
	if err != nil {
		return 5 * time.Second
	}

	// Check if in.png is still visible
	inPngVisible := false
	if len(b.targetsEntryVerify) > 0 {
		for _, verifyTarget := range b.targetsEntryVerify {
			_, _, found := b.searcher.FindTemplate(screenImg, verifyTarget.Image, constants.DefaultTolerance)
			if found {
				inPngVisible = true
				break
			}
		}
	}

	if !inPngVisible {
		// in.png disappeared - game has started!
		b.logFunc("Game started! Switching to Exit state.")
		b.entryWaitCount = 0
		b.setState(StateExitStep1)
		return 500 * time.Millisecond
	}

	// Still in lobby - check if we've waited too long
	if b.entryWaitCount >= 10 {
		b.logFunc("Waited too long in lobby (50s). Exiting to re-search...")

		// Click out.png to exit lobby
		if len(b.targetsEntryOut) > 0 {
			for _, outTarget := range b.targetsEntryOut {
				fx, fy, found := b.searcher.FindTemplate(screenImg, outTarget.Image, constants.DefaultTolerance)
				if found {
					b.performClick(outTarget.Name, fx, fy, outTarget.Image.Bounds().Dx(), outTarget.Image.Bounds().Dy())
					b.logFunc(fmt.Sprintf("Clicked exit button [%s]. Returning to search.", outTarget.Name))
					break
				}
			}
		}

		b.entryWaitCount = 0
		b.setState(StateSearchOpen)
		return constants.SearchScanInterval
	}

	b.debugFunc("[Waiting] in.png still visible, wait count=%d", b.entryWaitCount)
	return 5 * time.Second // Check again in 5 seconds
}

// getTargetByName finds a target by its name
func (b *GlobalBot) getTargetByName(name string) *Target {
	for i := range b.targetsEntry {
		if b.targetsEntry[i].Name == name {
			return &b.targetsEntry[i]
		}
	}
	return nil
}

func (b *GlobalBot) handleExitState() time.Duration {
	b.statusFunc("Status: Waiting for Exit...")
	
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return 10 * time.Second }

	for _, target := range b.targetsExit {
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
			time.Sleep(constants.WaitAfterClickNormal)
			b.logFunc("Exit verified. Switching to Search Flow.")
			b.setState(StateSearchOpen)
			return constants.SearchScanInterval
		}
	}
	return 5 * time.Second
}

func (b *GlobalBot) handleSearchOpenState() time.Duration {
	b.statusFunc("Status: Searching [Open List]...")
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return constants.SearchScanInterval }

	for _, target := range b.targetsSearchStep1 {
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
			time.Sleep(constants.WaitAfterClickNormal)
			b.setState(StateSearchSelect)
			return constants.WaitAfterClickNormal
		}
	}
	return constants.SearchScanInterval
}

func (b *GlobalBot) handleSearchSelectState() time.Duration {
	b.statusFunc("Status: Searching [Target Channel]...")
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return constants.SearchScanInterval }

	for _, target := range b.targetsSearchStep2 {
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.performClick(target.Name, fx, fy, target.Image.Bounds().Dx(), target.Image.Bounds().Dy())
			time.Sleep(constants.WaitAfterClickNormal)
			b.setState(StateSearchVerify)
			return constants.WaitAfterClickNormal
		}
	}
	return constants.SearchScanInterval
}

func (b *GlobalBot) handleSearchVerifyState() time.Duration {
	b.statusFunc("Status: Verifying Highlight...")
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil { return constants.SearchScanInterval }

	for _, target := range b.targetsSearchVerify {
		_, _, found := b.searcher.FindTemplate(screenImg, target.Image, constants.DefaultTolerance)
		if found {
			b.logFunc(fmt.Sprintf("Verified Highlight [%s]. Cycle Complete.", target.Name))
			time.Sleep(1 * time.Second)
			b.setState(StateEntry)
			return constants.SearchScanInterval
		}
	}
	return constants.SearchScanInterval
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
	b.targetsEntry, err = b.loadTargets("entry")
	if err != nil { return err }

	// Load entry verify targets (in.png for lobby detection)
	b.targetsEntryVerify, err = b.loadSpecificTarget("entry/verify", "in.png")
	if err != nil { b.debugFunc("Warning: No entry verify (in.png) target found.") }

	// Load entry out targets (out.png for exiting lobby)
	b.targetsEntryOut, err = b.loadSpecificTarget("entry/verify", "out.png")
	if err != nil { b.debugFunc("Warning: No entry out (out.png) target found.") }

	b.targetsExit, err = b.loadTargets("exit")
	if err != nil { b.debugFunc("Warning: No exit targets found.") }

	b.targetsSearchStep1, err = b.loadTargets("search/step1")
	if err != nil { b.debugFunc("Warning: No search step1 targets found.") }

	b.targetsSearchStep2, err = b.loadTargets("search/step2")
	if err != nil { b.debugFunc("Warning: No search step2 targets found.") }

	b.targetsSearchVerify, err = b.loadTargets("search/verify")
	if err != nil { b.debugFunc("Warning: No search verify targets found.") }

	b.logFunc(fmt.Sprintf("Loaded Assets: Entry=%d (in=%d, out=%d), Exit=%d, Search=%d/%d/%d",
		len(b.targetsEntry), len(b.targetsEntryVerify), len(b.targetsEntryOut), len(b.targetsExit),
		len(b.targetsSearchStep1), len(b.targetsSearchStep2), len(b.targetsSearchVerify)))
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
	
	if subDir == "entry" {
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
