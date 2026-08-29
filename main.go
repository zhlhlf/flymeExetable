package main

import (
	"embed"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:dist
var assets embed.FS

const (
	collapsedWindowWidth  = 58
	collapsedWindowHeight = 120
	setupWindowWidth      = 392
	setupWindowHeight     = 500
)

func main() {
	lock, ok, err := acquireSingleInstanceLock()
	if err != nil {
		println("Error:", err.Error())
		return
	}
	if !ok {
		return
	}
	defer lock.Release()

	app := NewApp()
	windowWidth := collapsedWindowWidth
	windowHeight := collapsedWindowHeight
	if cfg, err := readConfig(); err == nil && len(cfg.Folders) == 0 {
		windowWidth = setupWindowWidth
		windowHeight = setupWindowHeight
	}

	webviewDataPath := ""
	if dir, err := storeDir(); err == nil {
		webviewDataPath = filepath.Join(dir, "webview")
	}

	err = wails.Run(&options.App{
		Title:            "jczhl Filyme Launcher",
		Width:            windowWidth,
		Height:           windowHeight,
		MinWidth:         52,
		MinHeight:        72,
		Frameless:        true,
		DisableResize:    true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              true,
			WindowIsTranslucent:               true,
			BackdropType:                      windows.None,
			Theme:                             windows.Dark,
			DisableFramelessWindowDecorations: true,
			DisableWindowIcon:                 true,
			WebviewUserDataPath:               webviewDataPath,
			WebviewGpuIsDisabled:              false,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
