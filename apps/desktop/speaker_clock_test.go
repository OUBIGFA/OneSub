package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"testing"
)

func TestSpeakerAudioClockOffset(t *testing.T) {
	var info media.Info
	info.Format.StartTime = "10.0"
	info.Streams = []media.Stream{{CodecType: "video", StartTime: "10.0"}, {CodecType: "audio", StartTime: "10.25"}}
	offset, err := speakerAudioOffset(info)
	if err != nil || offset != 250 {
		t.Fatal(offset, err)
	}
	info.Format.StartTime = "-0.120"
	info.Streams[1].StartTime = "-0.035"
	offset, err = speakerAudioOffset(info)
	if err != nil || offset != 85 {
		t.Fatal(offset, err)
	}
	info.Format.StartTime = ""
	info.Streams[1].StartTime = ""
	offset, err = speakerAudioOffset(info)
	if err != nil || offset != 0 {
		t.Fatal(offset, err)
	}
	info.Streams[1].StartTime = "NaN"
	if _, err = speakerAudioOffset(info); err == nil {
		t.Fatal("accepted invalid clock")
	}
}
