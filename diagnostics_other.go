//go:build !windows

package main

import (
	"syscall"
	"time"
)

// processCPUTime returns total CPU time (kernel+user, summed across every
// core/thread the process has used) consumed since the process started, via
// getrusage - the portable-Unix counterpart to diagnostics_windows.go's
// GetProcessTimes. See that file's doc comment for how the delta around a
// measurement window is meant to be read.
func processCPUTime() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	utime := time.Duration(ru.Utime.Sec)*time.Second + time.Duration(ru.Utime.Usec)*time.Microsecond
	stime := time.Duration(ru.Stime.Sec)*time.Second + time.Duration(ru.Stime.Usec)*time.Microsecond
	return utime + stime
}
