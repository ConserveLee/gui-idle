package main

import (
	"github.com/ConserveLee/gui-idle/app/global"
	"github.com/ConserveLee/gui-idle/app/normal"
	"github.com/ConserveLee/gui-idle/app/tools"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
)

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Go Game Bot Toolset")
	myWindow.Resize(fyne.NewSize(500, 600))

	// Create tabs for different features
	tabs := container.NewAppTabs(
		container.NewTabItem("环球远征", global.NewGlobalExpeditionPanel()),
		container.NewTabItem("普通关卡", normal.NewNormalLevelPanel()),
		container.NewTabItem("工具箱", tools.NewToolsPanel(myWindow)),
	)

	tabs.SetTabLocation(container.TabLocationTop)

	myWindow.SetContent(tabs)
	myWindow.ShowAndRun()
}
