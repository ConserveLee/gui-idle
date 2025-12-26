package normal

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// NewNormalLevelPanel creates the UI panel for Normal Level AFK (Placeholder)
func NewNormalLevelPanel() fyne.CanvasObject {
	return container.NewCenter(
		container.NewVBox(
			widget.NewLabel("普通关卡挂机功能开发中..."),
			widget.NewIcon(nil), // Placeholder icon
			widget.NewButton("敬请期待 (TODO)", func() {}),
		),
	)
}