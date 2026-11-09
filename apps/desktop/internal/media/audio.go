package media

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Audio export deliberately selects one track and excludes cover art, subtitles
// and chapters. These portable playback files are not copies of the video container.
func audioArgs(options ComposeOptions, info Info, partial string) ([]string, error) {
	index, channels := -1, 0
	for _, stream := range info.Streams {
		if stream.CodecType == "audio" {
			index, channels = stream.Index, stream.Channels
			break
		}
	}
	if index < 0 {
		return nil, errors.New(locales.ErrAudioMissing())
	}
	var encoding []string
	switch strings.ToLower(filepath.Ext(options.Destination)) {
	case ".mp3":
		encoding = []string{"-c:a", "libmp3lame", "-b:a", "192k"}
	case ".m4a":
		encoding = []string{"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart"}
	case ".aac":
		encoding = []string{"-c:a", "aac", "-b:a", "192k"}
	case ".wav":
		encoding = []string{"-c:a", "pcm_s16le"}
	case ".flac":
		encoding = []string{"-c:a", "flac"}
	case ".ogg":
		encoding = []string{"-c:a", "libvorbis", "-q:a", "5"}
	case ".opus":
		encoding = []string{"-c:a", "libopus", "-b:a", "128k"}
	default:
		return nil, errors.New(locales.ErrAudioFormat())
	}
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-i", options.Source, "-map", fmt.Sprintf("0:%d", index), "-vn", "-sn", "-dn", "-map_metadata", "0", "-map_chapters", "-1", "-af", "asetpts=PTS-STARTPTS", "-ar", "48000"}
	// MP3 and Vorbis have portable mono/stereo playback; use the same channel
	// policy for every format so changing the container does not change the mix.
	if channels > 2 {
		args = append(args, "-ac", "2")
	}
	args = append(args, encoding...)
	if options.Sample {
		args = append(args, "-t", "5")
	}
	return append(args, "-progress", "pipe:1", "-nostats", "-y", partial), nil
}

// Container duration can include longer video or other audio tracks. Inspect
// packet timestamps of the selected track without decoding the video or storing
// an unbounded packet list. This also works when stream duration is absent (MKV).
func audioDuration(ctx context.Context, probe, source string) (int64, error) {
	first, end := math.Inf(1), math.Inf(-1)
	cmd := exec.Command(probe, "-v", "error", "-select_streams", "a:0", "-show_packets", "-show_entries", "packet=pts_time,duration_time", "-of", "compact=p=0:nk=0", source)
	err := jobs.Run(ctx, cmd, func(line string) {
		pts, duration := math.NaN(), float64(0)
		for _, part := range strings.Split(line, "|") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			n, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				continue
			}
			switch key {
			case "pts_time":
				pts = n
			case "duration_time":
				duration = max(0, n)
			}
		}
		if !math.IsNaN(pts) {
			first, end = min(first, pts), max(end, pts+duration)
		}
	})
	if err != nil {
		return 0, err
	}
	if math.IsInf(first, 0) || math.IsInf(end, 0) || end <= first {
		return 0, errors.New(locales.ErrComposeDuration())
	}
	return int64(math.Round((end - first) * 1000)), nil
}
