package screen

import (
	"fmt"
	"image"
	_ "image/png" // Register PNG decoder
	"math"
	"os"

	"github.com/kbinani/screenshot"
)

// Searcher handles screen capturing and template matching
type Searcher struct{
	DisplayIndex int
}

// NewSearcher creates a new instance
func NewSearcher() *Searcher {
	return &Searcher{
		DisplayIndex: 0, // Default to main display
	}
}

// SetDisplayID sets the target display index for capturing
func (s *Searcher) SetDisplayID(index int) {
	s.DisplayIndex = index
}

// LoadImage loads an image from the filesystem
func (s *Searcher) LoadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

// CaptureScreen returns the current screen image
func (s *Searcher) CaptureScreen() (image.Image, error) {
	// kbinani/screenshot handles multi-monitor bounds correctly
	bounds := screenshot.GetDisplayBounds(s.DisplayIndex)
	
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, fmt.Errorf("failed to capture screen %d: %v", s.DisplayIndex, err)
	}
	return img, nil
}

// FindTemplate searches for the 'template' image inside the 'screen' image.
// Returns x, y (top-left) and true if found.
func (s *Searcher) FindTemplate(screenImg, templateImg image.Image, tolerance float64) (int, int, bool) {
	sBounds := screenImg.Bounds()
	tBounds := templateImg.Bounds()

	sWidth, sHeight := sBounds.Dx(), sBounds.Dy()
	tWidth, tHeight := tBounds.Dx(), tBounds.Dy()

	if sWidth < tWidth || sHeight < tHeight {
		return 0, 0, false
	}

	// Helper to get color components normalized 0-255
	getRgb := func(img image.Image, x, y int) (r, g, b uint32) {
		c := img.At(x, y)
		r, g, b, _ = c.RGBA()
		// RGBA() returns 0-65535, we scale to 0-255
		return r >> 8, g >> 8, b >> 8
	}

	// We check the first pixel of the template against the screen for quick rejection
	tr0, tg0, tb0 := getRgb(templateImg, tBounds.Min.X, tBounds.Min.Y)

	// Iterate over the screen
	// Optimization: This is a basic sliding window.
	for y := sBounds.Min.Y; y <= sBounds.Max.Y-tHeight; y++ {
		for x := sBounds.Min.X; x <= sBounds.Max.X-tWidth; x++ {
			
			// Quick check: first pixel
			sr, sg, sb := getRgb(screenImg, x, y)
			if !colorSimilar(sr, sg, sb, tr0, tg0, tb0, tolerance) {
				continue
			}

			// Full check if first pixel matches
			if match(screenImg, templateImg, x, y, tolerance, getRgb) {
				return x, y, true
			}
		}
	}

	return 0, 0, false
}

func colorSimilar(r1, g1, b1, r2, g2, b2 uint32, tolerance float64) bool {
	diff := math.Sqrt(float64((r1-r2)*(r1-r2) + (g1-g2)*(g1-g2) + (b1-b2)*(b1-b2)))
	return diff <= tolerance
}

func match(screenImg, templateImg image.Image, sx, sy int, tolerance float64, getRgb func(image.Image, int, int) (uint32, uint32, uint32)) bool {
	tBounds := templateImg.Bounds()
	
	for ty := 0; ty < tBounds.Dy(); ty++ {
		for tx := 0; tx < tBounds.Dx(); tx++ {
			tr, tg, tb := getRgb(templateImg, tBounds.Min.X+tx, tBounds.Min.Y+ty)
			sr, sg, sb := getRgb(screenImg, sx+tx, sy+ty)

			if !colorSimilar(sr, sg, sb, tr, tg, tb, tolerance) {
				return false
			}
		}
	}
	return true
}
