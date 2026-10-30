package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Stage IDs of ComposeProgress; the interface says them in its language.
const (
	StageChooseOutput = "choose-output"
	StagePrepare      = "prepare"
	StageVerifyInput  = "verify-input"
	StageEncode       = "encode"
	StageMux          = "mux"
	StageVerifyOutput = "verify-output"
)

// ComposeOptions is frozen before execution. It follows SmartSub's separation
// between subtitle mode, stream mapping and encoder options, without its UI or
// its final-target -y overwrite behavior.
type ComposeOptions struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`    // hard or soft
	Quality     string `json:"quality"` // lossless, crf18, crf20 or crf23
	Sample      bool   `json:"sample"`
	Language    string `json:"language"`
	Revision    int64  `json:"revision"`
}
type ComposeReport struct {
	Version      int            `json:"version"`
	Source       string         `json:"source"`
	SourceSHA256 string         `json:"sourceSha256"`
	Output       string         `json:"output"`
	OutputSHA256 string         `json:"outputSha256"`
	ASSSHA256    string         `json:"subtitleSha256"`
	Revision     int64          `json:"revision"`
	Mode         string         `json:"mode"`
	Sample       bool           `json:"sample"`
	DurationMs   int64          `json:"durationMs"`
	Streams      map[string]int `json:"streams"`
	Chapters     int            `json:"chapters"`
	Checks       []string       `json:"checks"`
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

// chaptersPreserved checks the actual content, not just the number of chapter
// carriers. FFmpeg clips chapters to the requested five-second sample; a full
// export must retain every chapter's title and boundaries (within 1 ms).
func chaptersPreserved(source, output []Chapter, sample bool) bool {
	matched := 0
	for _, chapter := range source {
		start, startErr := strconv.ParseFloat(chapter.StartTime, 64)
		end, endErr := strconv.ParseFloat(chapter.EndTime, 64)
		if startErr != nil || endErr != nil || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || end < start {
			return false
		}
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

// ComposeArgs is the auditable stream plan. Existing audio/subtitle/attachment
// streams are never dropped silently. Chapter metadata has its own mapping;
// unsupported data streams still fail preflight. MP4 attachments require MKV.
func ComposeArgs(options ComposeOptions, info Info, assPath, partial string) ([]string, error) {
	if options.Mode != "hard" && options.Mode != "soft" {
		return nil, errors.New(locales.ErrComposeMode())
	}
	ext := strings.ToLower(filepath.Ext(options.Destination))
	if ext != ".mkv" && ext != ".mp4" {
		return nil, errors.New(locales.ErrComposeContainer())
	}
	start, _ := strconv.ParseFloat(info.Format.StartTime, 64)
	if math.Abs(start) > .001 {
		return nil, errors.New(locales.ErrComposeStartTime())
	}
	mainVideo := []Stream{}
	covers := []Stream{}
	audio := []Stream{}
	subs := []Stream{}
	attachments := []Stream{}
	for _, s := range compositionStreams(info) {
		switch s.CodecType {
		case "video":
			if s.Disposition.AttachedPic != 0 {
				covers = append(covers, s)
			} else {
				mainVideo = append(mainVideo, s)
			}
		case "audio":
			audio = append(audio, s)
		case "subtitle":
			subs = append(subs, s)
		case "attachment":
			attachments = append(attachments, s)
		default:
			return nil, errors.New(locales.ErrComposeStreamType(s.CodecType))
		}
	}
	if len(mainVideo) != 1 {
		return nil, errors.New(locales.ErrComposeMainVideo())
	}
	if options.Mode == "hard" && (mainVideo[0].ColorTransfer == "smpte2084" || mainVideo[0].ColorTransfer == "arib-std-b67") {
		return nil, errors.New(locales.ErrComposeHdr())
	}
	if ext == ".mp4" && len(attachments) > 0 {
		return nil, errors.New(locales.ErrComposeMp4Attachments())
	}
	if ext == ".mp4" {
		for _, s := range subs {
			switch s.CodecName {
			case "subrip", "ass", "ssa", "webvtt", "mov_text", "text":
			default:
				return nil, errors.New(locales.ErrComposeMp4Subtitles())
			}
		}
	}
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-i", options.Source}
	if options.Mode == "soft" {
		args = append(args, "-i", assPath)
	}
	args = append(args, "-map", fmt.Sprintf("0:%d", mainVideo[0].Index))
	for _, s := range covers {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	for _, s := range audio {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	if options.Mode == "soft" {
		args = append(args, "-map", "1:s:0")
	}
	for _, s := range subs {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	for _, s := range attachments {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	args = append(args, "-map_metadata", "0", "-map_chapters", "0", "-c", "copy")
	if options.Mode == "hard" {
		// The CRF levels and even-dimension handling mirror SmartSub. QP 0 is
		// x264's mathematically lossless mode: outside the burned subtitles the
		// frames equal the decoded source, in its own chroma layout where x264
		// encodes it. The filter uses task-local fixed names, never
		// shell-escaped user filenames.
		pixelFormat, rate := "yuv420p", []string{}
		switch options.Quality {
		case "lossless":
			pixelFormat, rate = losslessPixelFormat(mainVideo[0].PixFmt), []string{"-qp", "0"}
		case "crf18", "crf20", "crf23":
			rate = []string{"-crf", strings.TrimPrefix(options.Quality, "crf")}
		default:
			return nil, errors.New(locales.ErrComposeQuality())
		}
		filter := "ass=render.ass:fontsdir=fonts"
		if !strings.HasPrefix(pixelFormat, "yuv444") {
			filter += ",crop=trunc(iw/2)*2:trunc(ih/2)*2:0:0"
		}
		args = append(args, "-filter:v:0", filter, "-c:v:0", "libx264")
		args = append(args, rate...)
		args = append(args, "-preset", "medium", "-pix_fmt:v:0", pixelFormat, "-fps_mode:v:0", "passthrough")
	} else {
		if ext == ".mp4" {
			args = append(args, "-c:s", "mov_text")
		} else {
			args = append(args, "-c:s:0", "ass")
		}
		args = append(args, "-disposition:s:0", "default", "-metadata:s:s:0", "language="+containerLanguage(options.Language))
		for i, s := range subs {
			if s.Disposition.Default != 0 {
				args = append(args, fmt.Sprintf("-disposition:s:%d", i+1), "-default")
			}
		}
	}
	if options.Mode == "hard" && ext == ".mp4" && len(subs) > 0 {
		args = append(args, "-c:s", "mov_text")
	}
	if ext == ".mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	if options.Sample {
		args = append(args, "-t", "5")
	}
	return append(args, "-progress", "pipe:1", "-nostats", "-y", partial), nil
}

// losslessPixelFormat keeps the source's chroma layout and bit depth when
// libx264 can encode it losslessly; other layouts become 4:2:0.
func losslessPixelFormat(source string) string {
	switch source {
	case "yuv420p", "yuv422p", "yuv444p", "yuv420p10le", "yuv422p10le", "yuv444p10le":
		return source
	}
	return "yuv420p"
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
	if len(document.Report.MissingGlyphs) > 0 {
		return report, errors.New(locales.ErrMissingGlyphs(strings.Join(document.Report.MissingGlyphs, " ")))
	}
	info, err := Probe(ctx, probe, options.Source)
	if err != nil {
		return report, err
	}
	scratch, err := os.MkdirTemp(scratchRoot, "compose-")
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(scratch)
	assPath := filepath.Join(scratch, "render.ass")
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
	temporary, err := os.CreateTemp(filepath.Dir(options.Destination), ".onesub-*.partial"+filepath.Ext(options.Destination))
	if err != nil {
		return report, err
	}
	partial := temporary.Name()
	if err = temporary.Close(); err != nil {
		return report, err
	}
	defer os.Remove(partial)
	args, err := ComposeArgs(options, info, assPath, partial)
	if err != nil {
		return report, err
	}
	progress(ComposeProgress{Stage: StageVerifyInput, Fraction: -1})
	report.SourceSHA256, err = hashFile(ctx, options.Source)
	if err != nil {
		return report, err
	}
	command := exec.Command(ffmpeg, args...)
	command.Dir = scratch
	stage := StageEncode
	if options.Mode == "soft" {
		stage = StageMux
	}
	duration := info.DurationMs()
	if options.Sample && duration > 5000 {
		duration = 5000
	}
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
	if duration <= 0 || math.Abs(float64(output.DurationMs()-duration)) > 250 {
		return report, errors.New(locales.ErrComposeDuration())
	}
	if !chaptersPreserved(info.Chapters, output.Chapters, options.Sample) {
		return report, errors.New(locales.ErrComposeChapters())
	}
	// Full decode is intentionally stronger than merely finding streams in headers.
	if err = jobs.Run(ctx, exec.Command(ffmpeg, "-nostdin", "-v", "error", "-xerror", "-i", partial, "-map", "0:V:0", "-map", "0:a?", "-f", "null", "-"), nil); err != nil {
		return report, apperr.Wrap(locales.ErrComposeDecode(err.Error()), err)
	}
	report.OutputSHA256, err = hashFile(ctx, partial)
	if err != nil {
		return report, err
	}
	report.DurationMs = output.DurationMs()
	report.Streams = streamCounts(output.Streams)
	report.Chapters = len(output.Chapters)
	report.Checks = []string{"streams", "duration", "chapter titles and timings", "full audiovisual decode", "sha256"}
	if err = os.Link(partial, options.Destination); err != nil {
		return report, apperr.Wrap(locales.ErrComposePublish(err.Error()), err)
	}
	// The report is independent of the media file: report-write failure is surfaced
	// with the validated video preserved, never followed by another encode.
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return report, err
	}
	reportFile, err := os.OpenFile(options.Destination+".onesub.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return report, apperr.Wrap(locales.ErrComposeReport(err.Error()), err)
	}
	_, err = reportFile.Write(raw)
	if syncErr := reportFile.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := reportFile.Close(); err == nil {
		err = closeErr
	}
	return report, err
}
