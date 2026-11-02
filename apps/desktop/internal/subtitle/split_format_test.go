package subtitle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmartSplitStrictFormatsUseOwnTemplateAndPreserveMetadata(t *testing.T) {
	executable := os.Getenv("ONESUB_SUBTITLE_WORKER")
	if executable == "" {
		t.Skip("requires packaged subtitle worker")
	}
	text := "First we explain the complete workflow. Then we check every remaining operation before exporting the final subtitles."
	for _, format := range []string{"vtt", "ass"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			client := Client{Executable: executable, Scratch: root}
			original := "WEBVTT\n\nNOTE keep me\nmetadata\n\nother\n00:00.000 --> 00:01.000 align:end\nShort\n\nimportant\n00:02.000 --> 00:25.000 position:20% align:start\n" + text + "\n"
			if format == "ass" {
				original = "[Script Info]\nScriptType: v4.00+\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nComment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,keep me\nDialogue: 0,0:00:00.00,0:00:01.00,Other,,0,0,0,,Short\nDialogue: 2,0:00:02.00,0:00:25.00,Important,Alice,10,20,30,," + text + "\n"
			}
			path := filepath.Join(root, "input."+format)
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			doc, err := client.Read(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			r, err := SmartSplit(context.Background(), doc, "format", true)
			if err != nil || r.Changed != 1 {
				t.Fatal(r, err)
			}
			data, err := client.Write(context.Background(), r.Document)
			if err != nil {
				t.Fatal(err)
			}
			out := string(data)
			if !strings.Contains(out, "keep me") {
				t.Fatal("comment lost", out)
			}
			if format == "ass" {
				if strings.Count(out, ",Important,Alice,10,20,30,,") != r.Added+1 {
					t.Fatal("wrong ASS template", out)
				}
			} else {
				if strings.Count(out, "position:20% align:start") != r.Added+1 || strings.Count(out, "important-split-") != r.Added+1 {
					t.Fatal("VTT settings or unique IDs lost", out)
				}
			}
			output := filepath.Join(root, "output."+format)
			os.WriteFile(output, data, 0600)
			parsed, err := client.Read(context.Background(), output)
			if err != nil || len(parsed.Cues) != len(r.Document.Cues) {
				t.Fatal(err, parsed)
			}
			unchanged, _ := os.ReadFile(path)
			if string(unchanged) != original {
				t.Fatal("source overwritten")
			}
		})
	}
}
