package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// The window icon on Linux; macOS and Windows take theirs from the app bundle.
//
//go:embed build/appicon.png
var icon []byte

func main() {
	// Create an instance of the app structure
	app := NewApp()

	bar := app.appMenu()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Mardo - Markdown Browser",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.assetHandler(),
		},
		BackgroundColour: &options.RGBA{R: 15, G: 13, B: 26, A: 1},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		// Without Mac options Wails leaves the green zoom button disabled.
		Mac: &mac.Options{},
		Linux: &linux.Options{
			Icon:        icon,
			ProgramName: "mardo", // what the desktop matches the window to its launcher and icon by
		},
		Menu:       bar,
		OnStartup:  app.startup,
		OnDomReady: app.domReady,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
