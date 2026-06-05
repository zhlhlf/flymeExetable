package main

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unicode"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

type AppConfig struct {
	Folder         *string         `json:"folder"`
	WindowPosition *WindowPosition `json:"windowPosition"`
}

type WindowPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type LauncherItem struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Letter    string `json:"letter"`
	Extension string `json:"extension"`
	IsDir     bool   `json:"isDir"`
	Icon      string `json:"icon"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func storeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("无法定位用户主目录")
	}
	dir := filepath.Join(home, ".jczhl-filyme")
	return dir, os.MkdirAll(dir, 0755)
}

func configPath() (string, error) {
	dir, err := storeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func itemsCachePath() (string, error) {
	dir, err := storeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "items.json"), nil
}

func iconsDir() (string, error) {
	dir, err := storeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "icons")
	return path, os.MkdirAll(path, 0755)
}

func defaultConfig() AppConfig {
	return AppConfig{Folder: nil, WindowPosition: nil}
}

func readConfig() (AppConfig, error) {
	path, err := configPath()
	if err != nil {
		return defaultConfig(), err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := defaultConfig()
		return cfg, writeConfig(cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig(), err
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig(), nil
	}
	return cfg, nil
}

func writeConfig(cfg AppConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func firstLetter(name string) string {
	for _, r := range name {
		if r <= unicode.MaxASCII && unicode.IsLetter(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "#"
}

func displayName(path string, info os.FileInfo) string {
	base := info.Name()
	if !info.IsDir() {
		ext := filepath.Ext(base)
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" {
		return path
	}
	return base
}

func shouldCollect(path string, info os.FileInfo) bool {
	name := strings.ToLower(info.Name())
	if name == "desktop.ini" || name == "thumbs.db" || strings.HasPrefix(name, "~$") {
		return false
	}
	if info.IsDir() {
		return true
	}
	return true
}

func scanFolder(folder string) ([]LauncherItem, error) {
	root, err := filepath.Abs(folder)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, errors.New("文件夹不存在或不可访问：" + folder)
	}

	items := make([]LauncherItem, 0, 128)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil || !shouldCollect(path, info) {
			continue
		}

		name := displayName(path, info)
		items = append(items, LauncherItem{
			Name:      name,
			Path:      path,
			Letter:    firstLetter(name),
			Extension: strings.TrimPrefix(filepath.Ext(path), "."),
			IsDir:     info.IsDir(),
			Icon:      "",
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Letter == items[j].Letter {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].Letter < items[j].Letter
	})

	cache, err := itemsCachePath()
	if err == nil {
		if data, err := json.MarshalIndent(items, "", "  "); err == nil {
			_ = os.WriteFile(cache, data, 0644)
		}
	}

	return items, nil
}

func iconCachePath(path string) (string, error) {
	dir, err := iconsDir()
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(strings.ToLower(path)))
	return filepath.Join(dir, fmt.Sprintf("%x.png", sum)), nil
}

func iconDataURL(path string) string {
	cache, err := iconCachePath(path)
	if err != nil {
		return ""
	}

	if _, err := os.Stat(cache); err != nil {
		if err := extractWindowsIcon(path, cache); err != nil {
			return ""
		}
	}

	data, err := os.ReadFile(cache)
	if err != nil || len(data) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func extractWindowsIcon(sourcePath string, outputPath string) error {
	if runtime.GOOS != "windows" {
		return errors.New("only windows is supported")
	}

	ps := `Add-Type -AssemblyName System.Drawing; ` +
		`$p = $args[0]; $o = $args[1]; ` +
		`$icon = [System.Drawing.Icon]::ExtractAssociatedIcon($p); ` +
		`if ($null -eq $icon) { exit 2 }; ` +
		`$bmp = $icon.ToBitmap(); ` +
		`$bmp.Save($o, [System.Drawing.Imaging.ImageFormat]::Png); ` +
		`$bmp.Dispose(); $icon.Dispose();`

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps, sourcePath, outputPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func (a *App) GetConfig() (AppConfig, error) {
	return readConfig()
}

func (a *App) ChooseFolder() (string, error) {
	selected, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择要收集的应用文件夹",
	})
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", nil
	}
	cfg, _ := readConfig()
	cfg.Folder = &selected
	return selected, writeConfig(cfg)
}

func (a *App) SetFolder(folder string) (AppConfig, error) {
	cfg, _ := readConfig()
	cfg.Folder = &folder
	return cfg, writeConfig(cfg)
}

func (a *App) SaveWindowPosition(x int, y int) error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}
	cfg.WindowPosition = &WindowPosition{X: x, Y: y}
	return writeConfig(cfg)
}

func (a *App) ScanFolder(folder string) ([]LauncherItem, error) {
	return scanFolder(folder)
}

func (a *App) GetIcon(path string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("only windows is supported")
	}
	return iconDataURL(path), nil
}

func (a *App) OpenItem(path string) error {
	if runtime.GOOS != "windows" {
		return errors.New("only windows is supported")
	}
	cmd := exec.Command("cmd", "/C", "start", "", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
