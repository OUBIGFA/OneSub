package main

import (
	"time"

	"golang.org/x/sys/windows"
)

var getSubtitleDoubleClickTime = windows.NewLazySystemDLL("user32.dll").NewProc("GetDoubleClickTime")

// Match MyGo's Windows click counter and the user's accessibility setting.
func subtitleDoubleClickInterval() time.Duration {
	ms, _, _ := getSubtitleDoubleClickTime.Call()
	return time.Duration(ms)*time.Millisecond + time.Millisecond
}
