package tools

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kbinani/screenshot"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// NewToolsPanel creates the UI panel for utility tools
func NewToolsPanel(win fyne.Window) fyne.CanvasObject {
	// State
	selectedDisplay := 0
	
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
		if err == nil {
			selectedDisplay = id
		}
	})
	if len(displayOptions) > 0 {
		displaySelect.SetSelected(displayOptions[0])
	}

	// 2. Info Label
	infoLabel := widget.NewLabel("1. 选择屏幕\n2. 点击“截取并裁切”\n3. 在弹出的窗口中框选按钮\n4. 保存素材")
	infoLabel.Alignment = fyne.TextAlignCenter

	// 3. Action Buttons
	
	// The New Interactive Cropper
	cropBtn := widget.NewButton("截取并裁切 (Capture & Crop)", func() {
		// 1. Capture Full Screen
		bounds := screenshot.GetDisplayBounds(selectedDisplay)
		img, err := screenshot.CaptureRect(bounds)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}

		// 2. Open Cropper Window
		showCropperWindow(win, img)
	})
	cropBtn.Importance = widget.HighImportance

	openDirBtn := widget.NewButton("打开素材目录 (Open Assets)", func() {
		openDir("assets")
	})

	// Layout
	content := container.NewVBox(
		widget.NewLabel("选择屏幕:"),
		displaySelect,
		widget.NewSeparator(),
		infoLabel,
		layoutSpacer(),
		cropBtn,
		layoutSpacer(),
		widget.NewSeparator(),
		openDirBtn,
	)

	return content
}

func layoutSpacer() fyne.CanvasObject {
	return widget.NewLabel("") // rudimentary spacer
}

func openDir(path string) {
	var cmd *exec.Cmd
	absPath, _ := filepath.Abs(path)
	
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", absPath)
	case "windows":
		cmd = exec.Command("explorer", absPath)
	default:
		cmd = exec.Command("xdg-open", absPath)
	}
	cmd.Run()
}

func showCropperWindow(parent fyne.Window, fullImg image.Image) {
	w := fyne.CurrentApp().NewWindow("裁切素材 (Crop Template)")
	w.Resize(fyne.NewSize(800, 600))

	// Status label
	lbl := widget.NewLabel("请在图片上拖拽鼠标框选目标...")
	lbl.Alignment = fyne.TextAlignCenter

	// Confirm button (starts hidden or disabled)
	saveBtn := widget.NewButton("保存选区", nil)
	saveBtn.Disable()
	
	var currentSelection image.Rectangle

	// Cropper Widget
	cropper := NewCropperWidget(fullImg, func(rect image.Rectangle) {
		currentSelection = rect
		lbl.SetText(fmt.Sprintf("已选区: %v (点击保存)", rect))
		saveBtn.Enable()
	})

	saveBtn.OnTapped = func() {
		if currentSelection.Empty() {
			return
		}
		
		// Crop logic: SubImage
		subImg, ok := fullImg.(interface {
			SubImage(r image.Rectangle) image.Image
		})
		
		if !ok {
			dialog.ShowError(fmt.Errorf("image type does not support cropping"), w)
			return
		}
		
		finalImg := subImg.SubImage(currentSelection)
		
		// Show Save Dialog
		showSaveForm(w, finalImg)
	}

	content := container.NewBorder(
		nil, 
		container.NewVBox(lbl, saveBtn),
		nil, nil,
		cropper,
	)
	
	w.SetContent(content)
	w.Show()
}

func showSaveForm(win fyne.Window, img image.Image) {
	// Preview
	imageObj := canvas.NewImageFromImage(img)
	imageObj.FillMode = canvas.ImageFillContain
	imageObj.SetMinSize(fyne.NewSize(100, 100))

	// Form
	// Mapping friendly names to paths
	dirMap := map[string]string{
		"环球远征": "assets/global_targets",
		"普通关卡": "assets/normal_targets",
	}
	// Sorted keys for consistent UI order
	dirOptions := []string{"环球远征", "普通关卡"} 
	
	dirSelect := widget.NewSelect(dirOptions, nil)
	
	// Filename input
	nameEntry := widget.NewEntry()

	// Helper to update filename based on selection
	updateName := func(friendlyName string) {
		realDir, ok := dirMap[friendlyName]
		if !ok {
			return
		}
		// Ensure dir exists
		os.MkdirAll(realDir, 0755)
		
		next := getNextIndex(realDir)
		nameEntry.SetText(fmt.Sprintf("%d.png", next))
	}

	dirSelect.OnChanged = func(s string) {
		updateName(s)
	}
	
	// Init default
	dirSelect.SetSelected(dirOptions[0]) 
	// SetSelected triggers OnChanged, so updateName runs automatically

	content := container.NewVBox(
		widget.NewLabel("确认保存此素材?"),
		container.NewCenter(imageObj),
		widget.NewLabel("保存至 (Target Feature):"),
		dirSelect,
		widget.NewLabel("文件名 (Next Sequence):"),
		nameEntry,
	)

	dialog.ShowCustomConfirm("保存素材", "保存", "取消", content, func(confirm bool) {
		if !confirm {
			return
		}
		
		friendlyName := dirSelect.Selected
		realDir := dirMap[friendlyName]
		targetName := nameEntry.Text
		
		if targetName == "" {
			dialog.ShowError(fmt.Errorf("文件名不能为空"), win)
			return
		}
		
		targetPath := filepath.Join(realDir, targetName)
		
		// Ensure directory exists before saving
		if err := os.MkdirAll(realDir, 0755); err != nil {
			dialog.ShowError(err, win)
			return
		}
		
		f, err := os.Create(targetPath)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		defer f.Close()
		
		if err := png.Encode(f, img); err != nil {
			dialog.ShowError(err, win)
			return
		}
		
		dialog.ShowInformation("成功", fmt.Sprintf("已保存: %s\n(%s)", targetName, friendlyName), win)
		win.Close() // Close cropper window on success
	}, win)
}

// getNextIndex finds the next available number for *.png in dir
func getNextIndex(dir string) int {
	files, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	maxIdx := 0
	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		if idx, err := strconv.Atoi(name); err == nil {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
	}
	return maxIdx + 1
}