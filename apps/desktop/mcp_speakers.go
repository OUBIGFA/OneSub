package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type annotateSpeakersIn struct {
	MediaPath    string `json:"media_path" jsonschema:"absolute audio or video path"`
	SubtitlePath string `json:"subtitle_path" jsonschema:"absolute subtitle path; text and times are preserved"`
	SpeakerCount int    `json:"speaker_count,omitempty" jsonschema:"zero for automatic grouping, or the known number of speakers"`
	ShowSpeakers bool   `json:"show_speakers,omitempty" jsonschema:"prefix speaker names only in the new output"`
	OutputDir    string `json:"output_dir,omitempty" jsonschema:"absolute output directory"`
}

// Internal artifacts preserve structured labels without creating companion
// files in the media folder or flooding an MCP response with hour-long turns.
func (t toolset) keepSpeakerArtifact(doc subtitle.Document) (string, error) {
	if doc.Speakers == nil {
		return "", nil
	}
	dir := filepath.Join(t.dataRoot, "artifacts", "speakers")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	// Cue assignments may change while the acoustic analysis does not.
	path := filepath.Join(dir, artifactDigest(raw)+".json")
	return path, subtitle.Replace(path, raw)
}

func (s mcpServer) annotateSpeakers(ctx context.Context, req *mcp.CallToolRequest, in annotateSpeakersIn) (*mcp.CallToolResult, subtitleOut, error) {
	if err := mcpInputFile(in.MediaPath); err != nil {
		return nil, subtitleOut{}, err
	}
	if err := mcpInputFile(in.SubtitlePath); err != nil {
		return nil, subtitleOut{}, err
	}
	if in.OutputDir != "" && !filepath.IsAbs(in.OutputDir) {
		return nil, subtitleOut{}, errors.New("output_dir must be absolute")
	}
	prefs, err := s.prefs()
	if err != nil {
		return nil, subtitleOut{}, err
	}
	prefs.SpeakerCount = float64(in.SpeakerCount)
	doc, err := s.tools.worker.Read(ctx, in.SubtitlePath)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	state, err := s.tools.analyzeSpeakers(ctx, prefs, in.MediaPath, "", func(p recognitionStep) {
		progress(ctx, req, float64(p.Done), float64(p.Total), recognitionStepLabel(p))
	})
	if err != nil {
		return nil, subtitleOut{}, err
	}
	doc = subtitle.AttachSpeakers(doc, state)
	artifact, err := s.tools.keepSpeakerArtifact(doc)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	output := doc
	if in.ShowSpeakers {
		output = subtitle.WithSpeakerNames(doc)
	}
	raw, err := s.tools.worker.Write(ctx, output)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	path, err := publishRecognition(in.SubtitlePath, in.OutputDir, doc.Format, raw)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	return nil, subtitleOut{SubtitlePath: path, Cues: len(doc.Cues), SpeakerArtifact: artifact}, nil
}

func artifactDigest(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }

// A caller must carry the explicit annotation artifact into the next step.
// Match every text/time pair before restoring metadata; line numbers alone
// never authorize applying labels from a different subtitle revision.
func attachSpeakerArtifact(doc subtitle.Document, path string) (subtitle.Document, error) {
	if path == "" {
		return doc, nil
	}
	if err := mcpInputFile(path); err != nil {
		return doc, err
	}
	file, err := os.Open(path)
	if err != nil {
		return doc, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, 64*1024*1024+1))
	file.Close()
	if err != nil {
		return doc, err
	}
	if len(raw) > 64*1024*1024 {
		return doc, errors.New("speaker artifact too large")
	}
	var original subtitle.Document
	if err = json.Unmarshal(raw, &original); err != nil {
		return doc, err
	}
	if err = subtitle.ValidateSpeakerState(original.Speakers); err != nil {
		return doc, err
	}
	if original.Speakers == nil || len(original.Cues) != len(doc.Cues) {
		return doc, errors.New("speaker artifact does not match subtitles")
	}
	out := doc
	out.Cues = slices.Clone(doc.Cues)
	out.Speakers = original.Speakers
	for i, cue := range doc.Cues {
		from := original.Cues[i]
		if from.StartMs != cue.StartMs || from.EndMs != cue.EndMs || from.Text != cue.Text {
			return doc, errors.New("subtitles changed since speaker annotation")
		}
		out.Cues[i].Speaker = from.Speaker
		if !from.Translated {
			out.Cues[i].Words = from.Words
		}
	}
	return out, nil
}
