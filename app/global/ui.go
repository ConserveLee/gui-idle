package global

import (
	"fmt"
	"github.com/ConserveLee/gui-idle/internal/logger"

	"github.com/kbinani/screenshot"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

// NewGlobalExpeditionPanel creates the UI panel for Global Expedition AFK
func NewGlobalExpeditionPanel() fyne.CanvasObject {
	// --- Data Binding ---
	logData := binding.NewStringList()
	statusData := binding.NewString()
	statusData.Set("Status: Ready")
	
	appLogger := logger.NewAppLogger(logData)

	// --- Bot Initialization ---
	logCallback := func(msg string) { appLogger.Info(msg) }
	statusCallback := func(msg string) { statusData.Set(msg) }
	debugCallback := func(format string, args ...interface{}) { appLogger.Debug(format, args...) }

	// Use specific GlobalBot instead of generic engine.Bot
	gameBot := NewGlobalBot(logCallback, statusCallback, debugCallback)

	// --- UI Components ---

	// --- UI Components ---
	
	// 1. Screen Selector
	numDisplays := screenshot.NumActiveDisplays()
	var displayOptions []string
	for i := 0; i < numDisplays; i++ {
		bounds := screenshot.GetDisplayBounds(i)
		displayOptions = append(displayOptions, fmt.Sprintf("Display %d (%dx%d)", i, bounds.Dx(), bounds.Dy()))
	}
	if len(displayOptions) == 0 {
		displayOptions = []string{"Display 0 (Default)"}
	}
	
	displaySelect := widget.NewSelect(displayOptions, func(selected string) {
		var id int
		_, err := fmt.Sscanf(selected, "Display %d", &id)
		if err != nil { id = 0 }
		gameBot.SetDisplayID(id)
		appLogger.Info("Switched to Display %d", id)
	})
	if len(displayOptions) > 0 {
		displaySelect.SetSelected(displayOptions[0])
	}
	if displaySelect.Selected != "" {
		var id int
		fmt.Sscanf(displaySelect.Selected, "Display %d", &id)
		gameBot.SetDisplayID(id)
	}

	// 2. Status & Logs
	statusLabel := widget.NewLabelWithData(statusData)
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	logList := widget.NewListWithData(
		logData,
		func() fyne.CanvasObject { return widget.NewLabel("Log entry template") },
		func(i binding.DataItem, o fyne.CanvasObject) { o.(*widget.Label).Bind(i.(binding.String)) },
	)
	
	// Auto-scroll
	logData.AddListener(binding.NewDataListener(func() {
		list, _ := logData.Get()
		if len(list) > 0 { logList.ScrollToBottom() }
	}))

	// 3. Buttons
	startBtn := widget.NewButton("Start AFK", nil)
	stopBtn := widget.NewButton("Stop", nil)
	stopBtn.Disable()

	startBtn.OnTapped = func() {
		statusData.Set("Status: Running")
		startBtn.Disable()
		stopBtn.Enable()
		displaySelect.Disable()
		gameBot.Start()
	}

	stopBtn.OnTapped = func() {
		gameBot.Stop()
		stopBtn.Disable()
		startBtn.Enable()
		displaySelect.Enable()
	}

	// --- Layout ---
	controls := container.NewVBox(
		widget.NewLabel("环球远征挂机配置:"),
		container.NewHBox(widget.NewLabel("Screen:"), displaySelect),
		statusLabel,
		container.NewHBox(startBtn, stopBtn),
		widget.NewSeparator(),
		widget.NewLabel("运行日志:"),
	)

	return container.NewBorder(controls, nil, nil, nil, logList)
}

/*
TODO List for Global Expedition (Beta Status):
1. Error Handling: Add retry logic if targets are not found for a long time.
2. Statistics: Track number of levels completed, gold earned, etc.
3. State Machine: Handle unexpected popups or connection errors properly.
4. Performance: Optimize template matching frequency or region of interest.
*/
