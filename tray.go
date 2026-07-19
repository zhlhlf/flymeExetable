package main

import (
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ShowWindow restores the launcher window.
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowShow(a.ctx)
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.WindowSetAlwaysOnTop(a.ctx, true)
	wailsruntime.WindowSetAlwaysOnTop(a.ctx, false)
}

// HideWindow hides the launcher window.
func (a *App) HideWindow() {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowHide(a.ctx)
}
