// Package media probes real streams instead of trusting file extensions.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"strconv"
	"time"
)

type Stream struct {
	Index         int               `json:"index"`
	CodecType     string            `json:"codec_type"`
	CodecName     string            `json:"codec_name"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	Channels      int               `json:"channels"`
	TimeBase      string            `json:"time_base"`
	StartTime     string            `json:"start_time"`
	Tags          map[string]string `json:"tags"`
	ColorTransfer string            `json:"color_transfer"`
	Disposition   struct {
		Default     int `json:"default"`
		Forced      int `json:"forced"`
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}
type Info struct {
	Streams  []Stream `json:"streams"`
	Chapters []struct {
		ID int `json:"id"`
	} `json:"chapters"`
	Format struct {
		Duration   string `json:"duration"`
		StartTime  string `json:"start_time"`
		FormatName string `json:"format_name"`
	} `json:"format"`
}

func (i Info) HasVideo() bool {
	for _, stream := range i.Streams {
		if stream.CodecType == "video" {
			return true
		}
	}
	return false
}
func (i Info) DurationMs() int64 {
	seconds, _ := strconv.ParseFloat(i.Format.Duration, 64)
	return int64(seconds * 1000)
}

type limitedOutput struct{ bytes.Buffer }

func (o *limitedOutput) Write(p []byte) (int, error) {
	if o.Len()+len(p) > 1024*1024 {
		return 0, errors.New("媒体信息超过 1 MiB")
	}
	return o.Buffer.Write(p)
}

// Probe bounds both runtime and output; stderr is not surfaced with local paths.
func Probe(ctx context.Context, executable, path string) (Info, error) {
	var info Info
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := jobs.CommandContext(ctx, executable, "-v", "error", "-show_streams", "-show_format", "-show_chapters", "-of", "json", "-i", path)
	stdout, stderr := &limitedOutput{}, &limitedOutput{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return info, fmt.Errorf("无法探测媒体；请检查文件与 ffprobe：%w", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return info, fmt.Errorf("媒体信息无效：%w", err)
	}
	for _, stream := range info.Streams {
		if stream.CodecType == "audio" || stream.CodecType == "video" {
			return info, nil
		}
	}
	return info, errors.New("媒体没有可用的音频或视频流")
}
