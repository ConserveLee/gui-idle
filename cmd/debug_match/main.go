package main

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/ConserveLee/gui-idle/internal/constants"
)

func main() {
	screenImg, err := loadImage("debug_entry_screen.png")
	if err != nil {
		fmt.Printf("Failed to load screen: %v\n", err)
		return
	}
	fmt.Printf("Screen size: %dx%d\n", screenImg.Bounds().Dx(), screenImg.Bounds().Dy())
	fmt.Printf("Using MaxFailRate: %.0f%%\n", constants.MaxFailRate*100)

	templates := []string{"12.png", "13.png", "14.png", "11.png", "10.png"}

	for _, tplName := range templates {
		tplPath := filepath.Join("assets/global_targets/find_game/games", tplName)
		tplImg, err := loadImage(tplPath)
		if err != nil {
			fmt.Printf("Failed to load template %s: %v\n", tplName, err)
			continue
		}

		fmt.Printf("\n=== Testing %s (%dx%d) ===\n", tplName, tplImg.Bounds().Dx(), tplImg.Bounds().Dy())

		// Test with new percentage-based matching
		for _, tolerance := range []float64{60, 80} {
			matches := findAllTemplatesNew(screenImg, tplImg, tolerance)
			fmt.Printf("  Tolerance %.0f (%.0f%% fail allowed): %d matches", tolerance, constants.MaxFailRate*100, len(matches))
			if len(matches) > 0 {
				fmt.Printf(" -> %v", matches)
			}
			fmt.Println()
		}
	}
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func findAllTemplatesNew(screenImg, templateImg image.Image, tolerance float64) []image.Point {
	sBounds := screenImg.Bounds()
	tBounds := templateImg.Bounds()
	tWidth, tHeight := tBounds.Dx(), tBounds.Dy()

	var matches []image.Point

	getRgbAndAlpha := func(img image.Image, x, y int) (r, g, b, a uint32) {
		c := img.At(x, y)
		r, g, b, a = c.RGBA()
		return r >> 8, g >> 8, b >> 8, a >> 8
	}

	tr0, tg0, tb0, ta0 := getRgbAndAlpha(templateImg, tBounds.Min.X, tBounds.Min.Y)
	tr1, tg1, tb1, ta1 := getRgbAndAlpha(templateImg, tBounds.Min.X+tWidth/2, tBounds.Min.Y+tHeight/2)
	tr2, tg2, tb2, ta2 := getRgbAndAlpha(templateImg, tBounds.Max.X-1, tBounds.Max.Y-1)

	for y := sBounds.Min.Y; y <= sBounds.Max.Y-tHeight; y++ {
		for x := sBounds.Min.X; x <= sBounds.Max.X-tWidth; x++ {
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

			if matchNew(screenImg, templateImg, x, y, tolerance, getRgbAndAlpha) {
				matches = append(matches, image.Point{X: x, Y: y})
				x += tWidth / 2
			}
		}
	}

	return matches
}

func matchNew(screenImg, templateImg image.Image, sx, sy int, tolerance float64, getRgbAndAlpha func(image.Image, int, int) (uint32, uint32, uint32, uint32)) bool {
	tBounds := templateImg.Bounds()
	totalPixels := 0
	failedPixels := 0

	for ty := 0; ty < tBounds.Dy(); ty++ {
		for tx := 0; tx < tBounds.Dx(); tx++ {
			tr, tg, tb, ta := getRgbAndAlpha(templateImg, tBounds.Min.X+tx, tBounds.Min.Y+ty)
			if ta == 0 {
				continue
			}

			totalPixels++
			sr, sg, sb, _ := getRgbAndAlpha(screenImg, sx+tx, sy+ty)

			if !colorSimilar(sr, sg, sb, tr, tg, tb, tolerance) {
				failedPixels++
				if float64(failedPixels)/float64(totalPixels) > constants.MaxFailRate && totalPixels > 100 {
					return false
				}
			}
		}
	}

	if totalPixels == 0 {
		return false
	}
	return float64(failedPixels)/float64(totalPixels) <= constants.MaxFailRate
}

func colorSimilar(r1, g1, b1, r2, g2, b2 uint32, tolerance float64) bool {
	diff := math.Sqrt(float64((r1-r2)*(r1-r2) + (g1-g2)*(g1-g2) + (b1-b2)*(b1-b2)))
	return diff <= tolerance
}

