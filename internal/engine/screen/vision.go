package screen

import (
	"fmt"
	"image"
	_ "image/png" // Register PNG decoder for image.Decode
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
// Returns x, y (top-left) and true if found. (Backward compatibility wrapper)
func (s *Searcher) FindTemplate(screenImg, templateImg image.Image, tolerance float64) (int, int, bool) {
	matches := s.FindAllTemplates(screenImg, templateImg, tolerance)
	if len(matches) > 0 {
		return matches[0].X, matches[0].Y, true
	}
	return 0, 0, false
}

// FindAllTemplatesInROI searches for templates only within the specified ROI (Region of Interest).
// The ROI is specified in screen coordinates. Results are also in screen coordinates.
// If roi is empty (zero rect), falls back to full screen search.
func (s *Searcher) FindAllTemplatesInROI(screenImg, templateImg image.Image, roi image.Rectangle, tolerance float64) []image.Point {
	// If ROI is empty, do full screen search
	if roi.Empty() {
		return s.FindAllTemplates(screenImg, templateImg, tolerance)
	}

	sBounds := screenImg.Bounds()
	tBounds := templateImg.Bounds()
	tWidth, tHeight := tBounds.Dx(), tBounds.Dy()

	// Clamp ROI to screen bounds
	searchArea := roi.Intersect(sBounds)
	if searchArea.Empty() {
		return nil
	}

	// Ensure we have room for template matching
	if searchArea.Dx() < tWidth || searchArea.Dy() < tHeight {
		return nil
	}

	var matches []image.Point

	getRgbAndAlpha := func(img image.Image, x, y int) (r, g, b, a uint32) {
		c := img.At(x, y)
		r, g, b, a = c.RGBA()
		return r >> 8, g >> 8, b >> 8, a >> 8
	}

	// Key pixels for quick rejection
	tr0, tg0, tb0, ta0 := getRgbAndAlpha(templateImg, tBounds.Min.X, tBounds.Min.Y)
	tr1, tg1, tb1, ta1 := getRgbAndAlpha(templateImg, tBounds.Min.X+tWidth/2, tBounds.Min.Y+tHeight/2)
	tr2, tg2, tb2, ta2 := getRgbAndAlpha(templateImg, tBounds.Max.X-1, tBounds.Max.Y-1)

	// Search only within ROI
	for y := searchArea.Min.Y; y <= searchArea.Max.Y-tHeight; y++ {
		for x := searchArea.Min.X; x <= searchArea.Max.X-tWidth; x++ {
			// Quick checks
			if ta0 > 0 {
				sr, sg, sb, _ := getRgbAndAlpha(screenImg, x, y)
				if !colorSimilar(sr, sg, sb, tr0, tg0, tb0, tolerance) {
					continue
				}
			}
			if ta1 > 0 {
				sr, sg, sb, _ := getRgbAndAlpha(screenImg, x+tWidth/2, y+tHeight/2)
				if !colorSimilar(sr, sg, sb, tr1, tg1, tb1, tolerance) {
					continue
				}
			}
			if ta2 > 0 {
				sr, sg, sb, _ := getRgbAndAlpha(screenImg, x+tWidth-1, y+tHeight-1)
				if !colorSimilar(sr, sg, sb, tr2, tg2, tb2, tolerance) {
					continue
				}
			}

			// Full check
			if match(screenImg, templateImg, x, y, tolerance, getRgbAndAlpha) {
				matches = append(matches, image.Point{X: x, Y: y})
				x += tWidth / 2
			}
		}
	}

	return matches
}

// FindAllTemplates searches for ALL occurrences of 'template' in 'screen'.
// Returns a slice of coordinates (top-left).
func (s *Searcher) FindAllTemplates(screenImg, templateImg image.Image, tolerance float64) []image.Point {
	sBounds := screenImg.Bounds()
	tBounds := templateImg.Bounds()
	tWidth, tHeight := tBounds.Dx(), tBounds.Dy()

	var matches []image.Point

	// Helper to get color components normalized 0-255, plus Alpha
	getRgbAndAlpha := func(img image.Image, x, y int) (r, g, b, a uint32) {
		c := img.At(x, y)
		r, g, b, a = c.RGBA()
		return r >> 8, g >> 8, b >> 8, a >> 8
	}

	// We check a few key pixels of the template against the screen for quick rejection
	// Points: Top-Left, Center, Bottom-Right
	tr0, tg0, tb0, ta0 := getRgbAndAlpha(templateImg, tBounds.Min.X, tBounds.Min.Y)
	tr1, tg1, tb1, ta1 := getRgbAndAlpha(templateImg, tBounds.Min.X+tWidth/2, tBounds.Min.Y+tHeight/2)
	tr2, tg2, tb2, ta2 := getRgbAndAlpha(templateImg, tBounds.Max.X-1, tBounds.Max.Y-1)

	// Iterate over the screen
	// Optimization: This is a basic sliding window.
	for y := sBounds.Min.Y; y <= sBounds.Max.Y-tHeight; y++ {
		for x := sBounds.Min.X; x <= sBounds.Max.X-tWidth; x++ {

			// Quick checks
			if ta0 > 0 {
				sr, sg, sb, _ := getRgbAndAlpha(screenImg, x, y)
				if !colorSimilar(sr, sg, sb, tr0, tg0, tb0, tolerance) {
					continue
				}
			}
			if ta1 > 0 {
				sr, sg, sb, _ := getRgbAndAlpha(screenImg, x+tWidth/2, y+tHeight/2)
				if !colorSimilar(sr, sg, sb, tr1, tg1, tb1, tolerance) {
					continue
				}
			}
			if ta2 > 0 {
				sr, sg, sb, _ := getRgbAndAlpha(screenImg, x+tWidth-1, y+tHeight-1)
				if !colorSimilar(sr, sg, sb, tr2, tg2, tb2, tolerance) {
					continue
				}
			}

			// Full check
			if match(screenImg, templateImg, x, y, tolerance, getRgbAndAlpha) {
				// NOTE: Returns raw x,y (as requested in rollback)
				matches = append(matches, image.Point{X: x, Y: y})
				x += tWidth / 2
			}
		}
	}

	return matches
}

func colorSimilar(r1, g1, b1, r2, g2, b2 uint32, tolerance float64) bool {
	// Simple Euclidean distance in RGB space
	diff := math.Sqrt(float64((r1-r2)*(r1-r2) + (g1-g2)*(g1-g2) + (b1-b2)*(b1-b2)))
	return diff <= tolerance
}

func match(screenImg, templateImg image.Image, sx, sy int, tolerance float64, getRgbAndAlpha func(image.Image, int, int) (uint32, uint32, uint32, uint32)) bool {
	tBounds := templateImg.Bounds()
	
	for ty := 0; ty < tBounds.Dy(); ty++ {
		for tx := 0; tx < tBounds.Dx(); tx++ {
			tr, tg, tb, ta := getRgbAndAlpha(templateImg, tBounds.Min.X+tx, tBounds.Min.Y+ty)
			
			// Skip transparent pixels in template (act as wildcard)
			if ta == 0 {
				continue
			}
			
			sr, sg, sb, _ := getRgbAndAlpha(screenImg, sx+tx, sy+ty)

			if !colorSimilar(sr, sg, sb, tr, tg, tb, tolerance) {
				return false
			}
		}
	}
	return true
}
