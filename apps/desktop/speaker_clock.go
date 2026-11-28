package main

import (
	"errors"
	"math"
	"strconv"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
)

// Decoding WAV resets the first audio sample to zero. The player's zero is
// the container start, so delayed audio needs this offset exactly once.
func speakerAudioOffset(info media.Info) (int64, error) {
	parse := func(value string) (float64, error) {
		if value == "" || value == "N/A" {
			return 0, nil
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, errors.New("invalid media time base")
		}
		return v, nil
	}
	zero, err := parse(info.Format.StartTime)
	if err != nil {
		return 0, err
	}
	for _, stream := range info.Streams {
		if stream.CodecType == "audio" {
			start, err := parse(stream.StartTime)
			if err != nil {
				return 0, err
			}
			if stream.StartTime == "" || stream.StartTime == "N/A" {
				start = zero
			}
			delta := math.Round((start - zero) * 1000)
			if delta < 0 || delta > 9007199254740991 {
				return 0, errors.New("audio time base cannot be mapped to this media")
			}
			return int64(delta), nil
		}
	}
	return 0, errors.New("media has no audio track")
}
