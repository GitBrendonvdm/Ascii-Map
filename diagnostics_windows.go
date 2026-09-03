//go:build windows

package main

import (
	"time"
	"unsafe"
)

// kernel32 is the shared LazyDLL handle declared in console_windows.go;
// these are two more procs off the same DLL.
var (
	procGetProcessTimes   = kernel32.NewProc("GetProcessTimes")
	procGetCurrentProcess = kernel32.NewProc("GetCurrentProcess")
)

type winFiletime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

// asDuration converts a FILETIME (100-nanosecond intervals since a fixed
// epoch) into the elapsed span it represents when used as a value, not an
// absolute timestamp - exactly how GetProcessTimes' kernel/user outputs are
// meant to be read.
func (f winFiletime) asDuration() time.Duration {
	ticks := int64(f.HighDateTime)<<32 | int64(f.LowDateTime)
	return time.Duration(ticks * 100)
}

// processCPUTime returns total CPU time (kernel+user, summed across every
// core/thread the process has used) consumed since the process started.
// Taking the delta around a measurement window gives CPU-seconds spent
// during that window - for a concurrent algorithm running many goroutines
// across multiple cores at once, this can exceed wall-clock time, which is
// exactly the point: it reveals total resource cost, not just how fast the
// wall clock ticked while the work was spread across cores.
func processCPUTime() time.Duration {
	handle, _, _ := procGetCurrentProcess.Call()
	var creation, exit, kernelTime, userTime winFiletime
	procGetProcessTimes.Call(
		handle,
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	return kernelTime.asDuration() + userTime.asDuration()
}
