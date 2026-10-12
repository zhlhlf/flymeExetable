//go:build windows

package main

import (
	"syscall"
	"time"
	"unsafe"
)

// DWM 的系统圆角和窗口表面会在 HTML 圆角之外露出一块方形底色，
// 用 SetWindowRgn 把窗口直接裁成圆角矩形，四角像素不存在，底色无处显示。
const shapeCornerRadius = 25 // 与前端 CSS 的圆角保持一致（逻辑像素）

var (
	user32ShapeDLL                    = syscall.NewLazyDLL("user32.dll")
	procFindWindowW                   = user32ShapeDLL.NewProc("FindWindowW")
	procGetWindowRect                 = user32ShapeDLL.NewProc("GetWindowRect")
	procGetDpiForWindow               = user32ShapeDLL.NewProc("GetDpiForWindow")
	procSetWindowRgn                  = user32ShapeDLL.NewProc("SetWindowRgn")
	procSetWindowCompositionAttribute = user32ShapeDLL.NewProc("SetWindowCompositionAttribute")

	ntdllShapeDLL     = syscall.NewLazyDLL("ntdll.dll")
	procRtlGetVersion = ntdllShapeDLL.NewProc("RtlGetVersion")

	gdi32ShapeDLL          = syscall.NewLazyDLL("gdi32.dll")
	procCreateRoundRectRgn = gdi32ShapeDLL.NewProc("CreateRoundRectRgn")
	procDeleteObjectShape  = gdi32ShapeDLL.NewProc("DeleteObject")
)

type shapeRect struct {
	Left, Top, Right, Bottom int32
}

type accentPolicy struct {
	AccentState   int32
	AccentFlags   int32
	GradientColor uint32
	AnimationId   int32
}

type compositionAttribData struct {
	Attrib int32
	PvData unsafe.Pointer
	CbData uintptr
}

type osVersionInfoW struct {
	Size         uint32
	MajorVersion uint32
	MinorVersion uint32
	BuildNumber  uint32
	PlatformId   uint32
	CSDVersion   [128]uint16
}

const (
	accentStateTransparentGradient = 2
	compositionAttribAccentPolicy  = 19
	backdropMinBuild               = 22621
	acrylicTintColor               = 0x00000000
)

// watchWindowShape 轮询主窗口尺寸，尺寸变化时重新应用圆角裁剪
// （收起 58×120 / 展开 360×N / 设置 392×500 之间切换都会触发）。
func watchWindowShape() {
	className, _ := syscall.UTF16PtrFromString(wailsWindowClass)
	lastWidth, lastHeight := int32(0), int32(0)
	lastHwnd := uintptr(0)
	for {
		hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
		if hwnd != 0 {
			if hwnd != lastHwnd {
				applyAcrylicFallback(hwnd)
				lastHwnd = hwnd
			}
			var rect shapeRect
			if ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ret != 0 {
				width, height := rect.Right-rect.Left, rect.Bottom-rect.Top
				if width > 0 && height > 0 && (width != lastWidth || height != lastHeight) {
					if applyRoundRegion(hwnd, width, height) {
						lastWidth, lastHeight = width, height
					}
				}
			}
		}
		if hwnd == 0 {
			lastHwnd = 0
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func windowsBuildNumber() uint32 {
	var info osVersionInfoW
	info.Size = uint32(unsafe.Sizeof(info))
	if ret, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info))); ret != 0 {
		return 0
	}
	return info.BuildNumber
}

func applyAcrylicFallback(hwnd uintptr) {
	if windowsBuildNumber() >= backdropMinBuild {
		return
	}
	accent := accentPolicy{
		AccentState:   accentStateTransparentGradient,
		AccentFlags:   0x2,
		GradientColor: acrylicTintColor,
	}
	data := compositionAttribData{
		Attrib: compositionAttribAccentPolicy,
		PvData: unsafe.Pointer(&accent),
		CbData: unsafe.Sizeof(accent),
	}
	procSetWindowCompositionAttribute.Call(hwnd, uintptr(unsafe.Pointer(&data)))
}

func applyRoundRegion(hwnd uintptr, width, height int32) bool {
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		dpi = 96
	}
	// 裁剪半径略小于 CSS 圆角，保证裁剪边缘不露出窗口表面的底色。
	radius := int32(shapeCornerRadius)*int32(dpi)/96 - 2
	if radius < 2 {
		radius = 2
	}
	region, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(width+1), uintptr(height+1), uintptr(radius), uintptr(radius))
	if region == 0 {
		return false
	}
	if ret, _, _ := procSetWindowRgn.Call(hwnd, region, 1); ret == 0 {
		procDeleteObjectShape.Call(region)
		return false
	}
	return true
}
