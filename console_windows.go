//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle               = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

const (
	stdOutputHandle                 = ^uintptr(10) // STD_OUTPUT_HANDLE (-11), wrapped to uintptr
	enableVirtualTerminalProcessing = 0x0004
)

type coord struct {
	X, Y int16
}
type smallRect struct {
	Left, Top, Right, Bottom int16
}
type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

// enableANSI turns on VT100 escape-sequence processing for the current
// console, best-effort. Windows Terminal already has this on by default
// (a harmless no-op there); older conhost-hosted windows need it requested
// explicitly or color codes print as literal garbage text.
func enableANSI() {
	handle, _, _ := procGetStdHandle.Call(stdOutputHandle)
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return
	}
	procSetConsoleMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
}

// consoleWidth returns the current console's usable column width, or 0 if
// it can't be determined (e.g. stdout is redirected to a file).
func consoleWidth() int {
	handle, _, _ := procGetStdHandle.Call(stdOutputHandle)
	var info consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBufferInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0
	}
	w := int(info.Window.Right) - int(info.Window.Left) + 1
	if w <= 0 {
		return 0
	}
	return w
}
