package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/videoencoding"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Stage IDs of ComposeProgress; the interface says them in its language.
const (
	StageChooseOutput = "choose-output"
	StagePrepare      = "prepare"
	StageVerifyInput  = "verify-input"
	StageAnalyze      = "analyze"
	StageEncode       = "encode"
	StageMux          = "mux"
	StageVerifyOutput = "verify-output"
)

// ComposeOptions is frozen before execution. It follows SmartSub's separation
// between subtitle mode, stream mapping and encoder options, without its UI or
// its final-target -y overwrite behavior.
type ComposeOptions struct {
	Source      string  `json:"source"`
	Destination string  `json:"destination"`
	Mode        string  `json:"mode"`                  // hard, soft, compress or audio (no added subtitles)
	Quality     string  `json:"quality"`               // perceptual targets, or crf18/crf20/crf23 for cpu-h264
	Encoder     string  `json:"encoder,omitempty"`     // cpu (SVT-AV1 default), cpu-h264 or gpu
	ResolvedCRF float64 `json:"resolvedCrf,omitempty"` // CRF, or the hardware encoder's quality value
	// hardware is the detected GPU encoder; set by Compose, never by callers.
	hardware *hardwareEncoder
	Sample   bool   `json:"sample"`
	Language string `json:"language"`
	Revision int64  `json:"revision"`
}
type ComposeReport struct {
	Version      int                `json:"version"`
	Source       string             `json:"source"`
	SourceSHA256 string             `json:"sourceSha256"`
	SourceStart  float64            `json:"sourceStartSeconds"`
	Output       string             `json:"output"`
	OutputSHA256 string             `json:"outputSha256"`
	ASSSHA256    string             `json:"subtitleSha256"`
	Revision     int64              `json:"revision"`
	Mode         string             `json:"mode"`
	Sample       bool               `json:"sample"`
	DurationMs   int64              `json:"durationMs"`
	Streams      map[string]int     `json:"streams"`
	Chapters     int                `json:"chapters"`
	Compression  *CompressionReport `json:"compression,omitempty"`
	Checks       []string           `json:"checks"`
}

func streamCounts(streams []Stream) map[string]int {
	counts := map[string]int{}
	for _, s := range streams {
		counts[s.CodecType]++
	}
	return counts
}

// compositionStreams excludes only QuickTime text chapter carriers identified
// by FFmpeg's mov_read_chapters (data/bin_data with text or tx3g tag). Their
// content is already in Chapters and is preserved through -map_chapters, not
// raw stream copying: MP4 recreates a carrier, while MKV uses chapter metadata.
// A handler name alone is not evidence; unknown data and missing chapters stay
// in the plan and are rejected instead of being silently discarded.
func compositionStreams(info Info) []Stream {
	mov := strings.Contains(","+info.Format.FormatName+",", ",mov,")
	streams := make([]Stream, 0, len(info.Streams))
	for _, s := range info.Streams {
		if mov && len(info.Chapters) > 0 && s.CodecType == "data" && s.CodecName == "bin_data" && (s.CodecTag == "text" || s.CodecTag == "tx3g") {
			continue
		}
		streams = append(streams, s)
	}
	return streams
}

// chaptersPreserved compares chapter times on the same zero-based input
// timeline FFmpeg uses for every stream. Chapters before that origin are
// clipped/dropped, as in FFmpeg's chapter mapping; five-second samples clip
// again at their end. A container may renumber IDs, but not change titles.
func chaptersPreserved(source, output []Chapter, origin float64, sample bool) bool {
	matched := 0
	for _, chapter := range source {
		start, startErr := strconv.ParseFloat(chapter.StartTime, 64)
		end, endErr := strconv.ParseFloat(chapter.EndTime, 64)
		if startErr != nil || endErr != nil || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || end < start {
			return false
		}
		if end < origin {
			continue
		}
		start, end = max(0, start-origin), end-origin
		if sample {
			if start >= 5 {
				continue
			}
			end = min(end, 5)
		}
		if matched >= len(output) {
			return false
		}
		got := output[matched]
		gotStart, startErr := strconv.ParseFloat(got.StartTime, 64)
		gotEnd, endErr := strconv.ParseFloat(got.EndTime, 64)
		if startErr != nil || endErr != nil || !(math.Abs(gotStart-start) <= .001) || !(math.Abs(gotEnd-end) <= .001) || got.Tags["title"] != chapter.Tags["title"] {
			return false
		}
		matched++
	}
	return matched == len(output)
}

type streamPlan struct {
	ext         string
	mainVideo   Stream
	covers      []Stream
	audio       []Stream
	subs        []Stream
	attachments []Stream
}

func planCompositionStreams(options ComposeOptions, info Info) (streamPlan, error) {
	if options.Mode != "hard" && options.Mode != "soft" && options.Mode != "compress" {
		return streamPlan{}, errors.New(locales.ErrComposeMode())
	}
	ext := strings.ToLower(filepath.Ext(options.Destination))
	if ext != ".mkv" && ext != ".mp4" {
		return streamPlan{}, errors.New(locales.ErrComposeContainer())
	}
	plan := streamPlan{ext: ext}
	var mainVideo []Stream
	for _, s := range compositionStreams(info) {
		switch s.CodecType {
		case "video":
			if s.Disposition.AttachedPic != 0 {
				plan.covers = append(plan.covers, s)
			} else {
				mainVideo = append(mainVideo, s)
			}
		case "audio":
			plan.audio = append(plan.audio, s)
		case "subtitle":
			plan.subs = append(plan.subs, s)
		case "attachment":
			plan.attachments = append(plan.attachments, s)
		default:
			return streamPlan{}, errors.New(locales.ErrComposeStreamType(s.CodecType))
		}
	}
	if len(mainVideo) != 1 {
		return streamPlan{}, errors.New(locales.ErrComposeMainVideo())
	}
	plan.mainVideo = mainVideo[0]
	if options.Mode != "soft" && (plan.mainVideo.ColorTransfer == "smpte2084" || plan.mainVideo.ColorTransfer == "arib-std-b67") {
		return streamPlan{}, errors.New(locales.ErrComposeHdr())
	}
	if ext == ".mp4" && len(plan.attachments) > 0 {
		return streamPlan{}, errors.New(locales.ErrComposeMp4Attachments())
	}
	if ext == ".mp4" {
		for _, s := range plan.subs {
			switch s.CodecName {
			case "subrip", "ass", "ssa", "webvtt", "mov_text", "text":
			default:
				return streamPlan{}, errors.New(locales.ErrComposeMp4Subtitles())
			}
		}
	}
	return plan, nil
}

// ComposeArgs is the auditable stream plan. Existing audio/subtitle/attachment
// streams are never dropped silently. Chapter metadata has its own mapping;
// unsupported data streams still fail preflight. MP4 attachments require MKV.
func ComposeArgs(options ComposeOptions, info Info, assPath, partial string) ([]string, error) {
	if options.Mode == "audio" {
		return audioArgs(options, info, partial)
	}
	plan, err := planCompositionStreams(options, info)
	if err != nil {
		return nil, err
	}
	// FFmpeg's default input handling subtracts the container start from
	// all its streams together. Nonzero PTS is normal (AAC priming, edit
	// lists, MPEG-TS); neither per-stream PTS-STARTPTS nor a player seek
	// belongs here, since either can destroy the relative A/V timing.
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-i", options.Source}
	if options.Mode == "soft" {
		args = append(args, "-i", assPath)
	}
	args = append(args, "-map", fmt.Sprintf("0:%d", plan.mainVideo.Index))
	for _, s := range plan.covers {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	for _, s := range plan.audio {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	if options.Mode == "soft" {
		args = append(args, "-map", "1:s:0")
	}
	for _, s := range plan.subs {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	for _, s := range plan.attachments {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	args = append(args, "-map_metadata", "0", "-map_chapters", "0", "-c", "copy")
	if options.Mode != "soft" {
		// All encoders share stream mapping and validation. H.264 restores
		// fixed CRF without a perceptual search. Burn-in adds ASS using local
		// fixed names, never shell-escaped user filenames.
		if options.Encoder != "" && !videoencoding.ValidEncoder(options.Encoder) {
			return nil, errors.New(locales.ErrComposeMode())
		}
		preset, ok := CompressPresetFor(options.Quality)
		crf := options.ResolvedCRF
		if options.hardware != nil {
			preset, ok = gpuPresetFor(*options.hardware, options.Quality)
		}
		if options.Encoder == videoencoding.EncoderCPUH264 {
			crf, ok = videoencoding.H264CRF(options.Quality)
		}
		if !ok {
			return nil, errors.New(locales.ErrComposeQuality())
		}
		if crf <= 0 {
			crf = preset.InitialCRF
		}
		filters := []string{}
		if options.Mode == "hard" {
			filters = append(filters, "ass=render.ass:fontsdir=fonts")
		}
		filters = append(filters, "crop=trunc(iw/2)*2:trunc(ih/2)*2:0:0")
		args = append(args, "-filter:v:0", strings.Join(filters, ","))
		if options.Encoder == videoencoding.EncoderCPUH264 {
			args = append(args, "-c:v:0", "libx264", "-preset", "medium", "-crf", formatCRF(crf), "-pix_fmt:v:0", "yuv420p")
		} else if options.hardware != nil {
			args = append(args, options.hardware.videoArgs(crf, (plan.mainVideo.Width&^1)*(plan.mainVideo.Height&^1))...)
			if plan.ext == ".mp4" && options.hardware.Codec == "hevc" {
				// Apple players require the hvc1 sample entry for HEVC in MP4.
				args = append(args, "-tag:v:0", "hvc1")
			}
		} else {
			args = append(args,
				"-c:v:0", "libsvtav1",
				"-preset", strconv.Itoa(preset.Preset),
				"-pix_fmt:v:0", "yuv420p10le",
				"-svtav1-params", svtAV1Params(crf),
			)
		}
		args = append(args, "-fps_mode:v:0", "passthrough")
	} else {
		if plan.ext == ".mp4" {
			args = append(args, "-c:s", "mov_text")
		} else {
			args = append(args, "-c:s:0", "ass")
		}
		args = append(args, "-disposition:s:0", "default", "-metadata:s:s:0", "language="+containerLanguage(options.Language))
		for i, s := range plan.subs {
			if s.Disposition.Default != 0 {
				args = append(args, fmt.Sprintf("-disposition:s:%d", i+1), "-default")
			}
		}
	}
	if options.Mode != "soft" && plan.ext == ".mp4" && len(plan.subs) > 0 {
		args = append(args, "-c:s", "mov_text")
	}
	if plan.ext == ".mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	if options.Sample {
		args = append(args, "-t", "5")
	}
	// Negative decode timestamps from B frames are not a presentation
	// offset. Do not let the muxer shift the whole output a second time,
	// moving the new subtitle track away from the frozen subtitle times.
	return append(args, "-avoid_negative_ts", "disabled", "-progress", "pipe:1", "-nostats", "-y", partial), nil
}

func containerLanguage(tag string) string {
	values := map[string]string{"zh-CN": "zho", "zh-TW": "zho", "zh": "zho", "en": "eng", "ja": "jpn", "ko": "kor", "fr": "fra", "de": "deu", "es": "spa"}
	if value := values[tag]; value != "" {
		return value
	}
	return "und"
}

func hashFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buffer := make([]byte, 1024*1024)
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		n, e := f.Read(buffer)
		if n > 0 {
			h.Write(buffer[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComposeProgress separates measurable FFmpeg work from indeterminate hashing
// and validation. Fraction is -1 when duration is not known.
type ComposeProgress struct {
	Stage    string
	Fraction float64
}

func composeFraction(line string, durationMs int64) (float64, bool) {
	if !strings.HasPrefix(line, "out_time_us=") || durationMs <= 0 {
		return 0, false
	}
	us, err := strconv.ParseInt(strings.TrimPrefix(line, "out_time_us="), 10, 64)
	if err != nil || us < 0 {
		return 0, false
	}
	return math.Min(.999, float64(us)/float64(durationMs)/1000), true
}

// Compose writes only task-owned scratch/partial files until validation has
// passed. The final hard-link publish is atomic and refuses existing aliases.
func Compose(ctx context.Context, ffmpeg, probe, scratchRoot string, options ComposeOptions, document styles.Document, progress func(ComposeProgress)) (ComposeReport, error) {
	report := ComposeReport{Version: 1, Source: filepath.Base(options.Source), Output: filepath.Base(options.Destination), Revision: options.Revision, Mode: options.Mode, Sample: options.Sample}
	if _, err := os.Lstat(options.Destination); err == nil {
		return report, errors.New(locales.ErrComposeExists())
	} else if !os.IsNotExist(err) {
		return report, err
	}
	withSubtitles := options.Mode == "hard" || options.Mode == "soft"
	if withSubtitles && len(document.Report.MissingGlyphs) > 0 {
		return report, errors.New(locales.ErrMissingGlyphs(strings.Join(document.Report.MissingGlyphs, " ")))
	}
	info, err := Probe(ctx, probe, options.Source)
	if err != nil {
		return report, err
	}
	if info.Format.StartTime != "" {
		report.SourceStart, err = strconv.ParseFloat(info.Format.StartTime, 64)
		if err != nil || math.IsNaN(report.SourceStart) || math.IsInf(report.SourceStart, 0) {
			return report, errors.New(locales.ErrComposeStartTime())
		}
	}
	scratch, err := os.MkdirTemp(scratchRoot, "compose-")
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(scratch)
	assPath := ""
	if withSubtitles {
		assPath = filepath.Join(scratch, "render.ass")
		if err = os.WriteFile(assPath, []byte(document.ASS), 0600); err != nil {
			return report, err
		}
		digest := sha256.Sum256([]byte(document.ASS))
		report.ASSSHA256 = hex.EncodeToString(digest[:])
		fontDir := filepath.Join(scratch, "fonts")
		if err = os.Mkdir(fontDir, 0700); err != nil {
			return report, err
		}
		for index, font := range document.Fonts {
			actual, err := hashFile(ctx, font.Path)
			if err != nil {
				return report, err
			}
			if actual != font.SHA256 {
				return report, errors.New(locales.ErrFontChanged())
			}
			source, err := os.Open(font.Path)
			if err != nil {
				return report, err
			}
			target, err := os.OpenFile(filepath.Join(fontDir, fmt.Sprintf("font-%d%s", index, filepath.Ext(font.Path))), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				source.Close()
				return report, err
			}
			_, copyErr := io.Copy(target, source)
			source.Close()
			closeErr := target.Close()
			if copyErr != nil {
				return report, copyErr
			}
			if closeErr != nil {
				return report, closeErr
			}
		}
	}
	temporary, err := os.CreateTemp(filepath.Dir(options.Destination), ".onesub-*.partial"+filepath.Ext(options.Destination))
	if err != nil {
		return report, err
	}
	partial := temporary.Name()
	defer os.Remove(partial)
	if err = temporary.Close(); err != nil {
		return report, err
	}
	args, err := ComposeArgs(options, info, assPath, partial)
	if err != nil {
		return report, err
	}
	progress(ComposeProgress{Stage: StageVerifyInput, Fraction: -1})
	report.SourceSHA256, err = hashFile(ctx, options.Source)
	if err != nil {
		return report, err
	}
	stage := StageEncode
	if options.Mode == "soft" {
		stage = StageMux
	}
	duration := info.DurationMs()
	// Matroska's Segment Duration is an end timestamp, unlike MOV/MP4's
	// elapsed format duration. Its nonzero origin must be removed once
	// for progress and validation, just as FFmpeg removes it from PTS.
	if strings.Contains(info.Format.FormatName, "matroska") || strings.Contains(info.Format.FormatName, "webm") {
		duration -= int64(math.Round(report.SourceStart * 1000))
	}
	if options.Mode == "audio" {
		duration, err = audioDuration(ctx, probe, options.Source)
		if err != nil {
			return report, err
		}
	}
	if options.Mode == "compress" || options.Mode == "hard" {
		if options.Encoder == videoencoding.EncoderGPU {
			encoder, detectErr := DetectGPUEncoder(ctx, ffmpeg)
			if detectErr != nil {
				return report, detectErr
			}
			options.hardware = &encoder
		}
		if options.Encoder != videoencoding.EncoderCPUH264 && options.ResolvedCRF <= 0 {
			var resolved float64
			var compressionReport *CompressionReport
			var searchErr error
			if options.hardware != nil {
				plan, planErr := planCompositionStreams(options, info)
				if planErr != nil {
					return report, planErr
				}
				pixels := (plan.mainVideo.Width &^ 1) * (plan.mainVideo.Height &^ 1)
				resolved, compressionReport, searchErr = searchGPUCompressionQuality(ctx, ffmpeg, scratch, options, info, duration, *options.hardware, pixels, progress)
			} else {
				resolved, compressionReport, searchErr = searchCompressionCRF(ctx, ffmpeg, scratch, options, info, duration, progress)
			}
			report.Compression = compressionReport
			if searchErr != nil {
				return report, searchErr
			}
			options.ResolvedCRF = resolved
		}
		args, err = ComposeArgs(options, info, assPath, partial)
		if err != nil {
			return report, err
		}
	}
	if options.Sample && duration > 5000 {
		duration = 5000
	}
	command := exec.Command(ffmpeg, args...)
	command.Dir = scratch
	progress(ComposeProgress{Stage: stage, Fraction: 0})
	err = jobs.Run(ctx, command, func(line string) {
		if fraction, ok := composeFraction(line, duration); ok {
			progress(ComposeProgress{Stage: stage, Fraction: fraction})
		}
	})
	if err != nil {
		return report, err
	}
	progress(ComposeProgress{Stage: StageVerifyOutput, Fraction: -1})
	output, err := Probe(ctx, probe, partial)
	if err != nil {
		return report, err
	}
	expected := streamCounts(compositionStreams(info))
	if options.Mode == "audio" {
		expected = map[string]int{"audio": 1}
	}
	if options.Mode == "soft" {
		expected["subtitle"]++
	}
	actual := streamCounts(compositionStreams(output))
	for kind, count := range expected {
		if actual[kind] != count {
			return report, errors.New(locales.ErrComposeStreamCount(kind, count, actual[kind]))
		}
	}
	for kind, count := range actual {
		if expected[kind] != count {
			return report, errors.New(locales.ErrComposeStreamCount(kind, expected[kind], count))
		}
	}
	outputDuration := output.DurationMs()
	if options.Mode == "audio" {
		// Raw AAC has no duration header. The demuxer's bitrate estimate can
		// be far off after silence; validate actual packets, just as for input.
		outputDuration, err = audioDuration(ctx, probe, partial)
		if err != nil {
			return report, err
		}
	}
	if duration <= 0 || math.Abs(float64(outputDuration-duration)) > 250 {
		return report, errors.New(locales.ErrComposeDuration())
	}
	if options.Mode != "audio" && !chaptersPreserved(info.Chapters, output.Chapters, report.SourceStart, options.Sample) {
		return report, errors.New(locales.ErrComposeChapters())
	}
	// Full decode is intentionally stronger than merely finding streams in headers.
	decode := []string{"-nostdin", "-v", "error", "-xerror", "-i", partial}
	if options.Mode == "audio" {
		decode = append(decode, "-map", "0:a:0")
	} else {
		decode = append(decode, "-map", "0:V:0", "-map", "0:a?")
	}
	decode = append(decode, "-f", "null", "-")
	if err = jobs.Run(ctx, exec.Command(ffmpeg, decode...), nil); err != nil {
		return report, apperr.Wrap(locales.ErrComposeDecode(err.Error()), err)
	}
	report.OutputSHA256, err = hashFile(ctx, partial)
	if err != nil {
		return report, err
	}
	report.DurationMs = outputDuration
	report.Streams = streamCounts(output.Streams)
	report.Chapters = len(output.Chapters)
	report.Checks = []string{"streams", "duration", "chapter titles and timings", "full audiovisual decode", "sha256"}
	if (options.Mode == "compress" || options.Mode == "hard") && report.Compression != nil {
		checkName := "xpsnr crf search"
		if options.Encoder == videoencoding.EncoderGPU {
			checkName = "xpsnr gpu quality search"
		}
		report.Checks = append([]string{checkName}, report.Checks...)
	}
	if options.Mode == "audio" {
		report.Checks = []string{"audio stream only", "selected audio duration", "full audio decode", "sha256"}
	}
	if err = os.Link(partial, options.Destination); err != nil {
		return report, apperr.Wrap(locales.ErrComposePublish(err.Error()), err)
	}
	// Reports belong to the caller's recovery project, not beside a user's
	// video. The destination receives only the validated media file.
	return report, nil
}
