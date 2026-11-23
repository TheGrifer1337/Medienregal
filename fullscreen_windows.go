//go:build windows

package main

import "unsafe"

type windowRect struct{ Left, Top, Right, Bottom int32 }
type windowPoint struct{ X, Y int32 }

type windowPlacement struct {
	Length, Flags, ShowCmd uint32
	Min, Max               windowPoint
	Normal                 windowRect
}

type monitorInfo struct {
	Size          uint32
	Monitor, Work windowRect
	Flags         uint32
}

type fullscreenWindow struct {
	handle    uintptr
	active    bool
	style     uintptr
	placement windowPlacement
}

func (w *fullscreenWindow) set(enabled bool) {
	if enabled == w.active {
		return
	}
	const (
		styleIndex        = ^uintptr(15) // GWL_STYLE (-16)
		overlappedWindow  = 0x00CF0000
		preserveZOrder    = 0x0004
		preservePlacement = 0x0001 | 0x0002
		frameChanged      = 0x0020
		closestMonitor    = 2
	)
	if !enabled {
		user32.NewProc("SetWindowLongW").Call(w.handle, styleIndex, w.style)
		user32.NewProc("SetWindowPlacement").Call(w.handle, uintptr(unsafe.Pointer(&w.placement)))
		user32.NewProc("SetWindowPos").Call(w.handle, 0, 0, 0, 0, 0, frameChanged|preserveZOrder|preservePlacement)
		w.active = false
		return
	}

	w.placement.Length = uint32(unsafe.Sizeof(w.placement))
	ok, _, _ := user32.NewProc("GetWindowPlacement").Call(w.handle, uintptr(unsafe.Pointer(&w.placement)))
	if ok == 0 {
		return
	}
	monitor, _, _ := user32.NewProc("MonitorFromWindow").Call(w.handle, closestMonitor)
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, _ = user32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return
	}
	w.style, _, _ = user32.NewProc("GetWindowLongW").Call(w.handle, styleIndex)
	user32.NewProc("SetWindowLongW").Call(w.handle, styleIndex, w.style&^overlappedWindow)
	box := info.Monitor
	user32.NewProc("SetWindowPos").Call(w.handle, 0, uintptr(box.Left), uintptr(box.Top),
		uintptr(box.Right-box.Left), uintptr(box.Bottom-box.Top), frameChanged|preserveZOrder)
	w.active = true
}
