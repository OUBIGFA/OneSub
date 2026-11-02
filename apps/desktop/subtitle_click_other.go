//go:build !windows

package main

import "time"

// Other platforms are not part of the Windows 0.0.1 acceptance target.
func subtitleDoubleClickInterval() time.Duration { return 501 * time.Millisecond }
