package main

import (
	"TODO/backend"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := backend.NewApp()
	err := wails.Run(&options.App{
		Title:                    "TODO",
		Width:                    1200,
		Height:                   780,
		MinWidth:                 900,
		MinHeight:                620,
		DisableResize:            false,
		Fullscreen:               false,
		Frameless:                false,
		BackgroundColour:         &options.RGBA{R: 245, G: 246, B: 248, A: 1},
		AssetServer:              &assetserver.Options{Assets: assets},
		OnStartup:                app.Startup,
		OnShutdown:               app.Shutdown,
		EnableDefaultContextMenu: false,
		Bind:                     []interface{}{app},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
