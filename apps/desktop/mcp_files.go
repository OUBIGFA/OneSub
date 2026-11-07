package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// File tools share the desktop workers and publish new files. They do not
// mutate an open desktop project or require a separate persistent job system.
func (s mcpServer) addFileTools(server *mcp.Server) {
	no := false
	read := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: &no}
	}
	write := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, DestructiveHint: &no, OpenWorldHint: &no}
	}
	mcp.AddTool(server, &mcp.Tool{Name: "get_capabilities", Description: "Inspect local runtime availability, configured translation and downloaded recognition models before planning work; does not return credentials or contact a provider", Annotations: read("Inspect capabilities")}, s.capabilities)
	mcp.AddTool(server, &mcp.Tool{Name: "inspect_media", Description: "Inspect a local video or audio file with ffprobe; returns duration, streams and display aspect for planning recognition or composition", Annotations: read("Inspect media")}, s.inspectMedia)
	mcp.AddTool(server, &mcp.Tool{Name: "read_subtitles", Description: "Read a page of local SRT, VTT, ASS or SSA cues with zero-based indices and millisecond times; keep sha256 for edit_subtitles and use next_offset for subsequent pages", Annotations: read("Read subtitles")}, s.readSubtitles)
	mcp.AddTool(server, &mcp.Tool{Name: "edit_subtitles", Description: "Apply up to 500 text or timing edits to the exact subtitle snapshot returned by read_subtitles; save a new file of the same format atomically, preserving untouched cues and protected formatting", Annotations: write("Edit subtitles")}, s.editSubtitles)
	mcp.AddTool(server, &mcp.Tool{Name: "check_subtitles", Description: "Check subtitle serialization, timing and reading load locally; returns paginated findings with cue indices for edit_subtitles; these checks do not assess translation accuracy", Annotations: read("Check subtitles")}, s.checkSubtitles)
	mcp.AddTool(server, &mcp.Tool{Name: "convert_subtitles", Description: "Convert a local subtitle file to a new SRT, VTT, ASS or TXT file; ASS uses the same SmartSub style and font pipeline as OneSub, and existing ASS retains its layout", Annotations: write("Convert subtitles")}, s.convertSubtitles)
	mcp.AddTool(server, &mcp.Tool{Name: "compose_video", Description: "Create a new MP4 or MKV from a video and subtitles: soft embeds a selectable subtitle track, hard burns captions into frames; reuse OneSub style/font settings, preserve source streams and verify output; long files take minutes, cancellation stops workers", Annotations: write("Compose video")}, s.composeVideo)
}

type mcpModel struct {
	ID         string `json:"id"`
	Downloaded bool   `json:"downloaded"`
}
type capabilitiesOut struct {
	SubtitleWorker        bool       `json:"subtitle_worker"`
	StyleWorker           bool       `json:"style_worker"`
	MediaTools            bool       `json:"media_tools"`
	TranslationConfigured bool       `json:"translation_configured"`
	TranslationModel      string     `json:"translation_model"`
	TargetLanguage        string     `json:"target_language"`
	RecognitionMode       string     `json:"recognition_mode"`
	RecognitionModel      string     `json:"recognition_model"`
	Models                []mcpModel `json:"models"`
	SubtitleFormats       []string   `json:"subtitle_formats"`
	ComposeModes          []string   `json:"compose_modes"`
}

func (s mcpServer) capabilities(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, capabilitiesOut, error) {
	prefs, err := s.prefs()
	if err != nil {
		return nil, capabilitiesOut{}, err
	}
	available := func(path string) bool { info, err := os.Stat(path); return err == nil && info.Mode().IsRegular() }
	out := capabilitiesOut{SubtitleWorker: available(s.tools.worker.Executable), StyleWorker: available(s.tools.styles.Executable), MediaTools: available(s.tools.ffmpeg) && available(s.probePath()), TranslationConfigured: prefs.ProviderURL != "" && prefs.ProviderModel != "", TranslationModel: prefs.ProviderModel, TargetLanguage: prefs.TargetLanguage, RecognitionMode: prefs.RecognitionMode, RecognitionModel: prefs.AsrModel, Models: []mcpModel{}, SubtitleFormats: []string{"srt", "vtt", "ass", "ssa"}, ComposeModes: []string{"soft", "hard"}}
	for _, model := range models.Catalog() {
		if model.Kind == "asr" {
			out.Models = append(out.Models, mcpModel{model.ID, model.FilesPresent(s.tools.modelPath(prefs, model.ID))})
		}
	}
	return nil, out, nil
}

func (s mcpServer) probePath() string {
	if path := os.Getenv("ONESUB_FFPROBE"); path != "" {
		return path
	}
	return filepath.Join(filepath.Dir(s.tools.ffmpeg), "ffprobe.exe")
}

func mcpInputFile(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("input path must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("input must be a regular file")
	}
	return nil
}

// Reject unusable destinations before recognition, paid translation or encoding;
// Publish/Compose still enforce exclusive creation when the result is ready.
func mcpOutputFile(path string, formats ...string) error {
	if !filepath.IsAbs(path) {
		return errors.New("output_path must be absolute")
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	valid := false
	for _, format := range formats {
		valid = valid || ext == format
	}
	if !valid {
		return fmt.Errorf("output extension must be one of %s", strings.Join(formats, ", "))
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("output_path already exists; choose a new file")
	} else if !os.IsNotExist(err) {
		return err
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("output parent must be a directory")
	}
	return nil
}

type inspectMediaIn struct {
	MediaPath string `json:"media_path" jsonschema:"absolute path of the video or audio file"`
}
type inspectMediaOut struct {
	DurationMs  int64      `json:"duration_ms"`
	FrameAspect float64    `json:"frame_aspect"`
	Info        media.Info `json:"info"`
}

func (s mcpServer) inspectMedia(ctx context.Context, req *mcp.CallToolRequest, in inspectMediaIn) (*mcp.CallToolResult, inspectMediaOut, error) {
	if err := mcpInputFile(in.MediaPath); err != nil {
		return nil, inspectMediaOut{}, err
	}
	info, err := media.Probe(ctx, s.probePath(), in.MediaPath)
	return nil, inspectMediaOut{info.DurationMs(), info.FrameAspect(), info}, err
}

type readSubtitlesIn struct {
	SubtitlePath string `json:"subtitle_path" jsonschema:"absolute path of a subtitle file"`
	Offset       int    `json:"offset,omitempty" jsonschema:"zero-based starting cue or finding index; default 0"`
	Limit        int    `json:"limit,omitempty" jsonschema:"page size from 1 to 500; omitted or 0 means 100"`
}
type mcpCue struct {
	Index      int    `json:"index"`
	StartMs    int64  `json:"start_ms"`
	EndMs      int64  `json:"end_ms"`
	Text       string `json:"text"`
	ReadOnly   bool   `json:"read_only"`
	TextLocked bool   `json:"text_locked"`
	TimeLocked bool   `json:"time_locked"`
}
type readSubtitlesOut struct {
	Format     string   `json:"format"`
	SHA256     string   `json:"sha256"`
	Total      int      `json:"total"`
	NextOffset *int     `json:"next_offset,omitempty"`
	Cues       []mcpCue `json:"cues"`
}

func mcpPage(offset, limit int) (int, error) {
	if offset < 0 || limit < 0 || limit > 500 {
		return 0, errors.New("offset must be nonnegative and limit must be from 1 to 500")
	}
	if limit == 0 {
		limit = 100
	}
	return limit, nil
}

// Parse an immutable copy so the returned hash and cue indices always refer to
// the same bytes, even if another application saves the original while reading.
func (s mcpServer) readDocument(ctx context.Context, path string) (subtitle.Document, string, error) {
	if err := mcpInputFile(path); err != nil {
		return subtitle.Document{}, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return subtitle.Document{}, "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64*1024*1024+1))
	if err != nil {
		return subtitle.Document{}, "", err
	}
	if len(data) > 64*1024*1024 {
		return subtitle.Document{}, "", errors.New("subtitle file exceeds 64 MiB")
	}
	dir, err := os.MkdirTemp(s.tools.worker.Scratch, "mcp-read-")
	if err != nil {
		return subtitle.Document{}, "", err
	}
	defer os.RemoveAll(dir)
	snapshot := filepath.Join(dir, "input"+filepath.Ext(path))
	if err = os.WriteFile(snapshot, data, 0600); err != nil {
		return subtitle.Document{}, "", err
	}
	doc, err := s.tools.worker.Read(ctx, snapshot)
	// The shared writer preserves imported ASS event layout and rejects timing
	// edits; expose that restriction before an agent plans its edit batch.
	if doc.Format == "ass" || doc.Format == "ssa" {
		for i := range doc.Cues {
			doc.Cues[i].TimeLocked = true
		}
	}
	return doc, fmt.Sprintf("%x", sha256.Sum256(data)), err
}

func (s mcpServer) readSubtitles(ctx context.Context, req *mcp.CallToolRequest, in readSubtitlesIn) (*mcp.CallToolResult, readSubtitlesOut, error) {
	limit, err := mcpPage(in.Offset, in.Limit)
	if err != nil {
		return nil, readSubtitlesOut{}, err
	}
	doc, hash, err := s.readDocument(ctx, in.SubtitlePath)
	if err != nil {
		return nil, readSubtitlesOut{}, err
	}
	out := readSubtitlesOut{Format: doc.Format, SHA256: hash, Total: len(doc.Cues), Cues: []mcpCue{}}
	start := min(in.Offset, len(doc.Cues))
	end := start + min(limit, len(doc.Cues)-start)
	for i := start; i < end; i++ {
		cue := doc.Cues[i]
		out.Cues = append(out.Cues, mcpCue{i, cue.StartMs, cue.EndMs, cue.Text, cue.ReadOnly, cue.TextLocked, cue.TimeLocked})
	}
	if end < len(doc.Cues) {
		out.NextOffset = &end
	}
	return nil, out, nil
}

type mcpCueEdit struct {
	Index   int     `json:"index" jsonschema:"zero-based cue index from read_subtitles"`
	Text    *string `json:"text,omitempty" jsonschema:"replacement text; omit to preserve text"`
	StartMs *int64  `json:"start_ms,omitempty" jsonschema:"new inclusive start in milliseconds; omit to preserve"`
	EndMs   *int64  `json:"end_ms,omitempty" jsonschema:"new end in milliseconds; must exceed start"`
}
type editSubtitlesIn struct {
	SubtitlePath   string       `json:"subtitle_path"`
	OutputPath     string       `json:"output_path" jsonschema:"absolute new path with the same format as input"`
	ExpectedSHA256 string       `json:"expected_sha256" jsonschema:"sha256 from read_subtitles; refuse if the input has changed"`
	Edits          []mcpCueEdit `json:"edits" jsonschema:"1 to 500 distinct cue edits; the batch is all or nothing"`
}

func (s mcpServer) editSubtitles(ctx context.Context, req *mcp.CallToolRequest, in editSubtitlesIn) (*mcp.CallToolResult, subtitleOut, error) {
	if err := mcpOutputFile(in.OutputPath, strings.TrimPrefix(strings.ToLower(filepath.Ext(in.SubtitlePath)), ".")); err != nil {
		return nil, subtitleOut{}, err
	}
	if len(in.Edits) == 0 || len(in.Edits) > 500 {
		return nil, subtitleOut{}, errors.New("provide 1 to 500 edits")
	}
	doc, hash, err := s.readDocument(ctx, in.SubtitlePath)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	if in.ExpectedSHA256 != hash {
		return nil, subtitleOut{}, errors.New("subtitle changed; read_subtitles again before editing")
	}
	seen := map[int]bool{}
	for _, edit := range in.Edits {
		if edit.Index < 0 || edit.Index >= len(doc.Cues) || seen[edit.Index] {
			return nil, subtitleOut{}, errors.New("edit indices must be distinct and within the document")
		}
		seen[edit.Index] = true
		cue := &doc.Cues[edit.Index]
		if cue.ReadOnly || (edit.Text != nil && cue.TextLocked) || ((edit.StartMs != nil || edit.EndMs != nil) && cue.TimeLocked) {
			return nil, subtitleOut{}, fmt.Errorf("cue %d is protected", edit.Index)
		}
		if edit.Text == nil && edit.StartMs == nil && edit.EndMs == nil {
			return nil, subtitleOut{}, errors.New("each edit must change text or timing")
		}
		if edit.Text != nil {
			cue.Text = *edit.Text
		}
		if edit.StartMs != nil {
			cue.StartMs = *edit.StartMs
		}
		if edit.EndMs != nil {
			cue.EndMs = *edit.EndMs
		}
		if strings.TrimSpace(cue.Text) == "" || cue.StartMs < 0 || cue.EndMs <= cue.StartMs {
			return nil, subtitleOut{}, fmt.Errorf("cue %d has empty text or invalid timing", edit.Index)
		}
		cue.Words = nil // Imported word anchors cannot survive arbitrary text/time edits
	}
	data, err := s.tools.worker.Write(ctx, doc)
	if err == nil {
		err = subtitle.Publish(in.OutputPath, data)
	}
	return nil, subtitleOut{SubtitlePath: in.OutputPath, Cues: len(doc.Cues)}, err
}

type mcpFinding struct {
	Index int    `json:"index"`
	Code  string `json:"code"`
}
type checkSubtitlesOut struct {
	SHA256     string       `json:"sha256"`
	Cues       int          `json:"cues"`
	Total      int          `json:"total"`
	NextOffset *int         `json:"next_offset,omitempty"`
	Findings   []mcpFinding `json:"findings"`
}

func (s mcpServer) checkSubtitles(ctx context.Context, req *mcp.CallToolRequest, in readSubtitlesIn) (*mcp.CallToolResult, checkSubtitlesOut, error) {
	limit, err := mcpPage(in.Offset, in.Limit)
	if err != nil {
		return nil, checkSubtitlesOut{}, err
	}
	doc, hash, err := s.readDocument(ctx, in.SubtitlePath)
	if err != nil {
		return nil, checkSubtitlesOut{}, err
	}
	if _, err = s.tools.worker.Write(ctx, doc); err != nil {
		return nil, checkSubtitlesOut{}, err
	}
	findings := []mcpFinding{}
	// ASS events and imported files can be out of chronological order. Check
	// intervals by time while retaining original cue indices in the response.
	order := make([]int, len(doc.Cues))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return doc.Cues[order[i]].StartMs < doc.Cues[order[j]].StartMs })
	overlaps := make([]bool, len(doc.Cues))
	lastEnd := int64(0)
	for _, i := range order {
		overlaps[i] = doc.Cues[i].StartMs < lastEnd
		lastEnd = max(lastEnd, doc.Cues[i].EndMs)
	}
	for i, cue := range doc.Cues {
		load := subtitle.InspectReading(cue.Text, cue.EndMs-cue.StartMs)
		for _, check := range []struct {
			condition bool
			code      string
		}{{cue.StartMs < 0 || cue.EndMs <= cue.StartMs, "invalid_timing"}, {overlaps[i], "overlap"}, {load.TooLong, "too_long"}, {load.TooFast, "too_fast"}, {load.TooBrief, "too_brief"}} {
			if check.condition {
				findings = append(findings, mcpFinding{i, check.code})
			}
		}
	}
	start := min(in.Offset, len(findings))
	end := start + min(limit, len(findings)-start)
	out := checkSubtitlesOut{SHA256: hash, Cues: len(doc.Cues), Total: len(findings), Findings: findings[start:end]}
	if end < len(findings) {
		out.NextOffset = &end
	}
	return nil, out, nil
}

type convertSubtitlesIn struct {
	SubtitlePath string `json:"subtitle_path"`
	OutputPath   string `json:"output_path" jsonschema:"absolute new path ending in srt, vtt, ass or txt"`
}

// Preserve imported ASS event layouts; plain text formats use SmartSub's
// generator with the same saved primary style that the workbench uses.
func (s mcpServer) styledSubtitles(ctx context.Context, doc subtitle.Document, aspect float64) (styles.Document, error) {
	prefs, err := s.prefs()
	if err != nil {
		return styles.Document{}, err
	}
	var style styles.Style
	if prefs.LastStyle != nil {
		style = *prefs.LastStyle
	} else {
		catalog, err := s.tools.styles.Catalog(ctx)
		if err != nil {
			return styles.Document{}, err
		}
		style = catalog.DefaultStyle
	}
	if doc.Format == "ass" || doc.Format == "ssa" {
		data, err := s.tools.worker.Write(ctx, doc)
		if err != nil {
			return styles.Document{}, err
		}
		return s.tools.styles.Resolve(ctx, string(data), style)
	}
	return s.tools.styles.Build(ctx, doc.Cues, style, aspect)
}

func (s mcpServer) convertSubtitles(ctx context.Context, req *mcp.CallToolRequest, in convertSubtitlesIn) (*mcp.CallToolResult, subtitleOut, error) {
	if err := mcpOutputFile(in.OutputPath, "srt", "vtt", "ass", "txt"); err != nil {
		return nil, subtitleOut{}, err
	}
	doc, _, err := s.readDocument(ctx, in.SubtitlePath)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(in.OutputPath)), ".")
	var data []byte
	if format == "ass" {
		styled, styleErr := s.styledSubtitles(ctx, doc, 0)
		err = styleErr
		if err == nil && len(styled.Report.MissingGlyphs) > 0 {
			err = errors.New("selected font cannot render all subtitle characters")
		}
		data = []byte(styled.ASS)
	} else {
		doc.TargetFormat = format
		data, err = s.tools.worker.Write(ctx, doc)
	}
	if err == nil {
		err = subtitle.Publish(in.OutputPath, data)
	}
	return nil, subtitleOut{SubtitlePath: in.OutputPath, Cues: len(doc.Cues)}, err
}

type composeVideoIn struct {
	MediaPath    string `json:"media_path"`
	SubtitlePath string `json:"subtitle_path"`
	OutputPath   string `json:"output_path" jsonschema:"absolute new mp4 or mkv path"`
	Mode         string `json:"mode" jsonschema:"soft for selectable subtitles or hard for burned-in captions"`
	Quality      string `json:"quality,omitempty" jsonschema:"hard mode only: lossless, crf18, crf20 or crf23; default crf18"`
	Language     string `json:"language,omitempty" jsonschema:"subtitle language ID; omitted means und (undetermined)"`
	Sample       bool   `json:"sample,omitempty" jsonschema:"true creates a five-second sample before a full export"`
}
type composeVideoOut struct {
	VideoPath string              `json:"video_path"`
	Report    media.ComposeReport `json:"report"`
}

func (s mcpServer) composeVideo(ctx context.Context, req *mcp.CallToolRequest, in composeVideoIn) (*mcp.CallToolResult, composeVideoOut, error) {
	if err := mcpOutputFile(in.OutputPath, "mp4", "mkv"); err != nil {
		return nil, composeVideoOut{}, err
	}
	if err := mcpInputFile(in.MediaPath); err != nil {
		return nil, composeVideoOut{}, err
	}
	if in.Mode != "soft" && in.Mode != "hard" {
		return nil, composeVideoOut{}, errors.New("mode must be soft or hard")
	}
	if in.Quality == "" {
		in.Quality = "crf18"
	}
	if in.Quality != "lossless" && in.Quality != "crf18" && in.Quality != "crf20" && in.Quality != "crf23" {
		return nil, composeVideoOut{}, errors.New("unknown quality; use lossless, crf18, crf20 or crf23")
	}
	if in.Language == "" {
		in.Language = "und"
	}
	info, err := media.Probe(ctx, s.probePath(), in.MediaPath)
	if err != nil {
		return nil, composeVideoOut{}, err
	}
	doc, _, err := s.readDocument(ctx, in.SubtitlePath)
	if err != nil {
		return nil, composeVideoOut{}, err
	}
	styled, err := s.styledSubtitles(ctx, doc, info.FrameAspect())
	if err != nil {
		return nil, composeVideoOut{}, err
	}
	events := 0
	report, err := media.Compose(ctx, s.tools.ffmpeg, s.probePath(), s.tools.worker.Scratch, media.ComposeOptions{Source: in.MediaPath, Destination: in.OutputPath, Mode: in.Mode, Quality: in.Quality, Language: in.Language, Sample: in.Sample}, styled, func(p media.ComposeProgress) {
		events++
		progress(ctx, req, float64(events), 0, composeStageLabel(p.Stage))
	})
	return nil, composeVideoOut{VideoPath: in.OutputPath, Report: report}, err
}
