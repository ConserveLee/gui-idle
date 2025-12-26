package engine

import (
	"fmt"
	"github.com/ConserveLee/gui-idle/internal/engine/screen"
	"image"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-vgo/robotgo"
)

// BotStatus represents the current state of the bot
type BotStatus int

const (
	StatusStopped BotStatus = iota
	StatusRunning
)

// BotConfig holds the configuration for the automation
type BotConfig struct {
	AssetsDir string        // Directory containing target images
	Interval  time.Duration // Scan interval
}

type Target struct {
	Name  string
	Image image.Image
}

// Bot controls the automation logic
type Bot struct {
	Status    BotStatus
	Config    BotConfig
	
	// Callbacks for UI updates
	LogFunc    func(string) // For persistent logs (History)
	StatusFunc func(string) // For transient status (Label)
	DebugFunc  func(string, ...interface{}) // For console debug

	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	
	searcher  *screen.Searcher
	targets   []Target // Pre-loaded targets sorted by priority
}

// NewBot creates a new instance of the bot
func NewBot(logFunc func(string), statusFunc func(string), debugFunc func(string, ...interface{})) *Bot {
	return &Bot{
		Status:     StatusStopped,
		LogFunc:    logFunc,
		StatusFunc: statusFunc,
		DebugFunc:  debugFunc,
		stopChan:   make(chan struct{}),
		searcher:   screen.NewSearcher(),
		Config: BotConfig{
			AssetsDir: "assets/click",
			Interval:  1 * time.Second,
		},
	}
}

// loadAssets scans the configured directory for PNGs and loads them sorted by filename
func (b *Bot) loadAssets() error {
	files, err := filepath.Glob(filepath.Join(b.Config.AssetsDir, "*.png"))
	if err != nil {
		return err
	}
	
	b.targets = make([]Target, 0, len(files))
	
	for _, file := range files {
		img, err := b.searcher.LoadImage(file)
		if err != nil {
			b.DebugFunc("Failed to load asset %s: %v", file, err)
			continue
		}
		
		name := filepath.Base(file)
		b.targets = append(b.targets, Target{Name: name, Image: img})
		b.DebugFunc("Loaded target: %s", name)
	}
	
	if len(b.targets) == 0 {
		return fmt.Errorf("no valid PNG images found in %s", b.Config.AssetsDir)
	}
	
	return nil
}

// SetDisplayID sets which monitor the bot should scan
func (b *Bot) SetDisplayID(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.searcher.SetDisplayID(id)
}

// Start begins the automation loop
func (b *Bot) Start() {
	b.mu.Lock()
	if b.Status == StatusRunning {
		b.mu.Unlock()
		return
	}
	
	// Load assets before starting
	if err := b.loadAssets(); err != nil {
		b.LogFunc(fmt.Sprintf("Startup Error: %v", err))
		b.mu.Unlock()
		return
	}
	
	b.Status = StatusRunning
	b.stopChan = make(chan struct{}) // Re-make channel for restart ability
	b.mu.Unlock()

	b.LogFunc(fmt.Sprintf("Bot started. Loaded %d targets.", len(b.targets)))
	b.DebugFunc("Bot process started")
	b.wg.Add(1)

	go b.loop()
}

// Stop signals the automation loop to end
func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Status == StatusStopped {
		return
	}

	close(b.stopChan)
	b.wg.Wait() // Wait for loop to finish
	b.Status = StatusStopped
	b.LogFunc("Bot stopped.")
	b.StatusFunc("Status: Stopped")
}

// loop is the main logic loop
func (b *Bot) loop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.Config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopChan:
			return
		case <-ticker.C:
			b.process()
		}
	}
}

// process performs the detection and action
func (b *Bot) process() {
	// 1. Capture Screen
	screenImg, err := b.searcher.CaptureScreen()
	if err != nil {
		errMsg := fmt.Sprintf("Error capturing screen: %v", err)
		b.LogFunc(errMsg)
		b.DebugFunc(errMsg)
		return
	}
	
	// Update transient status (Scanning...)
	b.StatusFunc("Status: Scanning...")

	// 2. Iterate through targets by priority
	for _, target := range b.targets {
		// Use a tolerance of ~45 for RGB difference
		fx, fy, found := b.searcher.FindTemplate(screenImg, target.Image, 45.0)

		if found {
			// Log success
			msg := fmt.Sprintf("Found [%s] at: %d, %d", target.Name, fx, fy)
			b.LogFunc(msg)
			b.DebugFunc(msg)
			b.StatusFunc(fmt.Sprintf("Status: Clicking %s...", target.Name))

			// 3. Click logic
			robotgo.MoveMouse(fx, fy)
			robotgo.Click("left")
			time.Sleep(10 * time.Millisecond)
			robotgo.Click("left")
			
			b.LogFunc("Action: Double Click Executed.")
			
			// Stop processing other targets in this cycle (priority mode)
			return
		}
	}

	// If loop finishes without return, nothing was found
	b.StatusFunc("Status: Scanning... (No targets found)")
}
