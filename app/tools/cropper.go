package tools

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// CropperWidget is a custom widget that displays an image and allows selecting a rectangular region.
type CropperWidget struct {
	widget.BaseWidget
	
	// State
	originalImg image.Image
	startPos    fyne.Position
	currentPos  fyne.Position
	isDragging  bool
	
	// UI Elements
	raster      *canvas.Image
	selection   *canvas.Rectangle
	
	// Callback
	OnSelected func(rect image.Rectangle)
}

func NewCropperWidget(img image.Image, onSelected func(image.Rectangle)) *CropperWidget {
	c := &CropperWidget{
		originalImg: img,
		OnSelected:  onSelected,
	}
	c.ExtendBaseWidget(c)
	
	c.raster = canvas.NewImageFromImage(img)
	c.raster.ScaleMode = canvas.ImageScalePixels // Crucial: No interpolation/smoothing
	c.raster.FillMode = canvas.ImageFillContain
	
	// Selection rectangle with semi-transparent fill
	c.selection = canvas.NewRectangle(color.RGBA{R: 255, G: 0, B: 0, A: 60}) // Semi-transparent Red
	c.selection.StrokeColor = color.RGBA{R: 255, G: 0, B: 0, A: 255}         // Solid Red Stroke
	c.selection.StrokeWidth = 2
	c.selection.Hide()
	
	return c
}

func (c *CropperWidget) CreateRenderer() fyne.WidgetRenderer {
	return &cropperRenderer{
		cropper: c,
		objects: []fyne.CanvasObject{c.raster, c.selection},
	}
}

// Mouse events
func (c *CropperWidget) Dragged(e *fyne.DragEvent) {
	if !c.isDragging {
		c.isDragging = true
		c.startPos = e.Position.Subtract(e.Dragged) // Approx start
		c.selection.Show() // Explicitly show
	}
	c.currentPos = e.Position
	c.Refresh()
}

func (c *CropperWidget) DragEnd() {
	c.isDragging = false
	c.Refresh()
	c.onDragEndLogic()
	// Do not hide here, keep selection visible
}

func (c *CropperWidget) Tapped(e *fyne.PointEvent) {
	c.startPos = e.Position
	c.currentPos = e.Position
	c.selection.Hide() // Hide on click (reset)
	c.Refresh()
}

// Cursor
func (c *CropperWidget) Cursor() desktop.Cursor {
	return desktop.CrosshairCursor
}

// Helpers
func (c *CropperWidget) calculateSelectionRect() fyne.Size {
	return fyne.NewSize(0,0) // Placeholder
}

func (c *CropperWidget) calculateImageRect() fyne.Size {
	wBound := c.Size().Width
	hBound := c.Size().Height
	
	imgW := float32(c.originalImg.Bounds().Dx())
	imgH := float32(c.originalImg.Bounds().Dy())
	aspect := imgW / imgH
	
	viewAspect := wBound / hBound
	
	var drawW, drawH float32
	var offX, offY float32
	
	if viewAspect > aspect {
		// View is wider than image: Fit Height
		drawH = hBound
		drawW = drawH * aspect
		offX = (wBound - drawW) / 2
		offY = 0
	} else {
		// View is taller than image: Fit Width
		drawW = wBound
		drawH = drawW / aspect
		offX = 0
		offY = (hBound - drawH) / 2
	}
	
	return fyne.NewSize(drawW, drawH).Add(fyne.NewPos(offX, offY))
}

// --- Renderer ---

type cropperRenderer struct {
	cropper *CropperWidget
	objects []fyne.CanvasObject
}

func (r *cropperRenderer) Layout(s fyne.Size) {
	// Layout the image to fill
	r.objects[0].Resize(s)
	r.objects[0].Move(fyne.NewPos(0, 0))
	
	// Layout the selection box
	c := r.cropper
	
	// Always calculate geometry, visibility is handled by widget state
	minX := min(c.startPos.X, c.currentPos.X)
	minY := min(c.startPos.Y, c.currentPos.Y)
	maxX := max(c.startPos.X, c.currentPos.X)
	maxY := max(c.startPos.Y, c.currentPos.Y)
	
	r.objects[1].Move(fyne.NewPos(minX, minY))
	r.objects[1].Resize(fyne.NewSize(maxX-minX, maxY-minY))
}

func (r *cropperRenderer) MinSize() fyne.Size {
	return fyne.NewSize(100, 100)
}

func (r *cropperRenderer) Refresh() {
	// Explicitly update geometry during drag/refresh events
	c := r.cropper
	minX := min(c.startPos.X, c.currentPos.X)
	minY := min(c.startPos.Y, c.currentPos.Y)
	maxX := max(c.startPos.X, c.currentPos.X)
	maxY := max(c.startPos.Y, c.currentPos.Y)
	
	r.objects[1].Move(fyne.NewPos(minX, minY))
	r.objects[1].Resize(fyne.NewSize(maxX-minX, maxY-minY))
	
	canvas.Refresh(r.cropper)
}

func (r *cropperRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *cropperRenderer) Destroy() {}

// Utils
func min(a, b float32) float32 { if a < b { return a }; return b }
func max(a, b float32) float32 { if a > b { return a }; return b }

// Structure helper
type rect struct {
	Position1 fyne.Position
	Width     float32
	Height    float32
}

// Helper to calculate image bounds (x, y, w, h)
func (c *CropperWidget) calculateImageRectStruct() rect {
	wBound := c.Size().Width
	hBound := c.Size().Height
	
	if wBound == 0 || hBound == 0 {
		return rect{}
	}
	
	imgW := float32(c.originalImg.Bounds().Dx())
	imgH := float32(c.originalImg.Bounds().Dy())
	aspect := imgW / imgH
	
	viewAspect := wBound / hBound
	
	var drawW, drawH float32
	var offX, offY float32
	
	if viewAspect > aspect {
		// View is wider: Fit Height
		drawH = hBound
		drawW = drawH * aspect
		offX = (wBound - drawW) / 2
		offY = 0
	} else {
		// View is taller: Fit Width
		drawW = wBound
		drawH = drawW / aspect
		offX = 0
		offY = (hBound - drawH) / 2
	}
	
	return rect{
		Position1: fyne.NewPos(offX, offY),
		Width:     drawW,
		Height:    drawH,
	}
}

// Re-implement DragEnd logic with struct
func (c *CropperWidget) onDragEndLogic() {
	if c.OnSelected == nil { return }
	
	imgRect := c.calculateImageRectStruct()
	
	// Selection Rect
	minX := min(c.startPos.X, c.currentPos.X)
	minY := min(c.startPos.Y, c.currentPos.Y)
	maxX := max(c.startPos.X, c.currentPos.X)
	maxY := max(c.startPos.Y, c.currentPos.Y)
	
	selX := minX
	selY := minY
	selW := maxX - minX
	selH := maxY - minY
	
	// Intersection
	interX := max(imgRect.Position1.X, selX)
	interY := max(imgRect.Position1.Y, selY)
	interRight := min(imgRect.Position1.X+imgRect.Width, selX+selW)
	interBottom := min(imgRect.Position1.Y+imgRect.Height, selY+selH)
	
	interW := interRight - interX
	interH := interBottom - interY
	
	if interW <= 0 || interH <= 0 {
		return
	}
	
	// Map to Pixel
	scaleX := float32(c.originalImg.Bounds().Dx()) / imgRect.Width
	scaleY := float32(c.originalImg.Bounds().Dy()) / imgRect.Height
	
	relX := interX - imgRect.Position1.X
	relY := interY - imgRect.Position1.Y
	
	// SubImage Rect
	// Note: image.Rect takes (x0, y0, x1, y1)
	finalRect := image.Rect(
		int(relX * scaleX),
		int(relY * scaleY),
		int((relX + interW) * scaleX),
		int((relY + interH) * scaleY),
	)
	
	// Ensure bounds are safe (sometimes float math overshoots)
	finalRect = finalRect.Intersect(c.originalImg.Bounds())
	
	c.OnSelected(finalRect)
}