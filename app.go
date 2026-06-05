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
	"golang.org/x/text/encoding/simplifiedchinese"
)

type App struct {
	ctx context.Context
}

type AppConfig struct {
	Folder         *string         `json:"folder"`
	Folders        []string        `json:"folders"`
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
	return AppConfig{Folder: nil, Folders: []string{}, WindowPosition: nil}
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
	if len(cfg.Folders) == 0 && cfg.Folder != nil && *cfg.Folder != "" {
		cfg.Folders = []string{*cfg.Folder}
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
		if unicode.IsSpace(r) {
			continue
		}
		if r <= unicode.MaxASCII {
			if unicode.IsLetter(r) {
				return strings.ToUpper(string(r))
			}
			return "#"
		}
		if initial, ok := chinesePinyinInitial(r); ok {
			return initial
		}
		return "#"
	}
	return "#"
}

func chinesePinyinInitial(r rune) (string, bool) {
	if r < '\u4e00' || r > '\u9fff' {
		return "", false
	}

	encoded, err := simplifiedchinese.GBK.NewEncoder().String(string(r))
	if err != nil || len(encoded) < 2 {
		return "#", true
	}

	code := int(encoded[0])<<8 + int(encoded[1])
	ranges := []struct {
		start   int
		initial string
	}{
		{0xB0A1, "A"}, {0xB0C5, "B"}, {0xB2C1, "C"}, {0xB4EE, "D"},
		{0xB6EA, "E"}, {0xB7A2, "F"}, {0xB8C1, "G"}, {0xB9FE, "H"},
		{0xBBF7, "J"}, {0xBFA6, "K"}, {0xC0AC, "L"}, {0xC2E8, "M"},
		{0xC4C3, "N"}, {0xC5B6, "O"}, {0xC5BE, "P"}, {0xC6DA, "Q"},
		{0xC8BB, "R"}, {0xC8F6, "S"}, {0xCBFA, "T"}, {0xCDDA, "W"},
		{0xCEF4, "X"}, {0xD1B9, "Y"}, {0xD4D1, "Z"},
	}

	for i := len(ranges) - 1; i >= 0; i-- {
		if code >= ranges[i].start {
			return ranges[i].initial, true
		}
	}
	return "#", true
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
	seenPaths := make(map[string]bool)
	for _, scanRoot := range scanRoots(root) {
		entries, err := os.ReadDir(scanRoot)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			path := filepath.Join(scanRoot, entry.Name())
			pathKey := strings.ToLower(filepath.Clean(path))
			if seenPaths[pathKey] {
				continue
			}
			seenPaths[pathKey] = true

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

func scanFolders(folders []string) ([]LauncherItem, error) {
	allItems := make([]LauncherItem, 0, 256)
	seenPaths := make(map[string]bool)

	for _, folder := range folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}
		items, err := scanFolder(folder)
		if err != nil {
			continue
		}
		for _, item := range items {
			key := strings.ToLower(filepath.Clean(item.Path))
			if seenPaths[key] {
				continue
			}
			seenPaths[key] = true
			allItems = append(allItems, item)
		}
	}

	sort.SliceStable(allItems, func(i, j int) bool {
		if allItems[i].Letter == allItems[j].Letter {
			return strings.ToLower(allItems[i].Name) < strings.ToLower(allItems[j].Name)
		}
		return allItems[i].Letter < allItems[j].Letter
	})

	cache, err := itemsCachePath()
	if err == nil {
		if data, err := json.MarshalIndent(allItems, "", "  "); err == nil {
			_ = os.WriteFile(cache, data, 0644)
		}
	}

	return allItems, nil
}

func scanRoots(root string) []string {
	roots := []string{root}
	home, _ := os.UserHomeDir()
	public := os.Getenv("PUBLIC")
	userDesktop := filepath.Clean(filepath.Join(home, "Desktop"))
	publicDesktop := filepath.Clean(filepath.Join(public, "Desktop"))

	// Windows 资源管理器里的“桌面”会合并用户桌面和公共桌面。
	// 例如 OpenVPN GUI.lnk 常见位置是 C:\Users\Public\Desktop。
	if public != "" && strings.EqualFold(filepath.Clean(root), userDesktop) {
		if info, err := os.Stat(publicDesktop); err == nil && info.IsDir() {
			roots = append(roots, publicDesktop)
		}
	}

	return roots
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
	return selected, nil
}

func (a *App) SetFolder(folder string) (AppConfig, error) {
	cfg, _ := readConfig()
	cfg.Folder = &folder
	cfg.Folders = []string{folder}
	return cfg, writeConfig(cfg)
}

func (a *App) SetFolders(folders []string) (AppConfig, error) {
	cleaned := normalizeFolders(folders)
	cfg, _ := readConfig()
	cfg.Folders = cleaned
	cfg.Folder = nil
	if len(cleaned) > 0 {
		cfg.Folder = &cleaned[0]
	}
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

func (a *App) ScanFolders(folders []string) ([]LauncherItem, error) {
	return scanFolders(folders)
}

func normalizeFolders(folders []string) []string {
	result := make([]string, 0, len(folders))
	seen := make(map[string]bool)
	for _, folder := range folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}
		abs, err := filepath.Abs(folder)
		if err != nil {
			abs = folder
		}
		key := strings.ToLower(filepath.Clean(abs))
		if seen[key] {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			seen[key] = true
			result = append(result, abs)
		}
	}
	return result
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
