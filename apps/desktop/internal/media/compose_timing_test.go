package media

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Real muxers, not mocked probe values: AAC priming and B-frame DTS are kept,
// with a second audio track 400 ms late. MP4 and MKV have different duration
// semantics when the timeline starts after zero.
func TestRealNonzeroStartComposition(t *testing.T) {
	worker, ffmpeg, probe := compositionTestTools(t)
	root := t.TempDir()
	document := compositionTestDocument(t, worker, root, 16.0/9)
	for _, inputExt := range []string{"mp4", "mkv"} {
		for _, offset := range []string{"0.033", "2", "-0.25"} {
			source := filepath.Join(root, "source-"+offset+"."+inputExt)
			args := []string{"-v", "error"}
			if offset == "-0.25" {
				// Matroska cannot represent negative video blocks: keep video
				// PTS positive and audio preroll negative, to test a valid origin
				// instead of a fixture whose video timestamps were lost at mux.
				args = append(args, "-itsoffset", "0.3")
			}
			args = append(args, "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30:duration=3", "-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-itsoffset", "0.4", "-f", "lavfi", "-i", "sine=frequency=880:duration=3", "-map", "0:v", "-map", "1:a", "-map", "2:a", "-c:v", "libx264", "-c:a", "aac", "-output_ts_offset", offset, "-avoid_negative_ts", "disabled", source)
			if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
				t.Fatalf("timing fixture: %v %s", err, out)
			}
			input, err := Probe(context.Background(), probe, source)
			if err != nil {
				t.Fatal(err)
			}
			origin, _ := strconv.ParseFloat(input.Format.StartTime, 64)
			if offset != "-0.25" && origin <= 0 {
				t.Fatal("fixture did not have a positive start")
			}
			for _, mode := range []string{"hard", "soft"} {
				for _, ext := range []string{"mp4", "mkv"} {
					t.Run(fmt.Sprintf("%s/%s/%s/%s", inputExt, offset, mode, ext), func(t *testing.T) {
						outputDir, scratch := t.TempDir(), t.TempDir()
						target := filepath.Join(outputDir, "result."+ext)
						opts := ComposeOptions{Source: source, Destination: target, Mode: mode, Quality: "lossless", Language: "en"}
						report, err := Compose(context.Background(), ffmpeg, probe, scratch, opts, document, func(ComposeProgress) {})
						if err != nil {
							t.Fatal(err)
						}
						if report.SourceStart != origin || report.Streams["audio"] != 2 || report.OutputSHA256 == "" {
							t.Fatal(report)
						}
						assertOnlyFiles(t, outputDir, filepath.Base(target))
						assertOnlyFiles(t, scratch)
						// Stream starts must retain relative timing. MP4 edit lists
						// can hide preroll on negative inputs; check nonnegative inputs
						// to avoid mistaking a decoder preroll packet for presentation.
						if offset != "-0.25" {
							output, err := Probe(context.Background(), probe, target)
							if err != nil {
								t.Fatal(err)
							}
							for index := 0; index < 3; index++ {
								before, _ := strconv.ParseFloat(input.Streams[index].StartTime, 64)
								after, _ := strconv.ParseFloat(output.Streams[index].StartTime, 64)
								if math.Abs(after-(before-origin)) > .034 {
									t.Fatalf("stream %d shifted: before=%g origin=%g after=%g", index, before, origin, after)
								}
							}
						}
						if mode == "soft" {
							assertSubtitlePacketTime(t, probe, target, .1)
						}
					})
				}
			}
		}
	}
}

func assertOnlyFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(names) {
		t.Fatalf("unexpected task files in %s: %v", dir, entries)
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func assertSubtitlePacketTime(t *testing.T, probe, path string, start float64) {
	t.Helper()
	raw, err := exec.Command(probe, "-v", "error", "-select_streams", "s:0", "-show_packets", "-show_entries", "packet=pts_time,size", "-of", "json", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Packets []struct {
			PTS  string `json:"pts_time"`
			Size string `json:"size"`
		}
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	for _, packet := range result.Packets {
		size, _ := strconv.Atoi(packet.Size)
		if size <= 2 {
			continue
		} // mov_text empty gap packet, not a subtitle
		got, err := strconv.ParseFloat(packet.PTS, 64)
		if err != nil || math.Abs(got-start) > .001 {
			t.Fatalf("subtitle PTS=%s, want %g", packet.PTS, start)
		}
		return
	}
	t.Fatal("no timed subtitle packet")
}

func TestCompositionFailureAndCancellationLeaveNoSidecars(t *testing.T) {
	worker, ffmpeg, probe := compositionTestTools(t)
	root := t.TempDir()
	source := chapterMediaFixture(t, ffmpeg, root)
	document := compositionTestDocument(t, worker, root, 16.0/9)
	for _, scenario := range []string{"invalid-quality", "cancel-encode", "cancel-validation"} {
		t.Run(scenario, func(t *testing.T) {
			outDir, scratch := t.TempDir(), t.TempDir()
			opts := ComposeOptions{Source: source, Destination: filepath.Join(outDir, "result.mp4"), Mode: "hard", Quality: "crf23"}
			if scenario == "invalid-quality" {
				opts.Quality = "invalid"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := Compose(ctx, ffmpeg, probe, scratch, opts, document, func(p ComposeProgress) {
				if scenario == "cancel-encode" && p.Stage == StageEncode || scenario == "cancel-validation" && p.Stage == StageVerifyOutput {
					cancel()
				}
			})
			if err == nil {
				t.Fatal("unsuccessful task published a result")
			}
			assertOnlyFiles(t, outDir)
			assertOnlyFiles(t, scratch)
		})
	}
}

func TestChapterTimesShareSourceOrigin(t *testing.T) {
	input := []Chapter{{StartTime: "2", EndTime: "4", Tags: map[string]string{"title": "A"}}, {StartTime: "4", EndTime: "8", Tags: map[string]string{"title": "B"}}}
	output := []Chapter{{StartTime: "0", EndTime: "2", Tags: input[0].Tags}, {StartTime: "2", EndTime: "5", Tags: input[1].Tags}}
	if !chaptersPreserved(input, output, 2, true) || chaptersPreserved(input, output, 0, true) {
		t.Fatal("chapters did not share normalized PTS")
	}
}

// Chapters on a nonzero Matroska timeline move with the media, not with a
// second, independently zeroed clock. This uses a real FFmetadata import.
func TestRealNonzeroChapterTimeline(t *testing.T) {
	worker, ffmpeg, probe := compositionTestTools(t)
	root := t.TempDir()
	base := chapterMediaFixture(t, ffmpeg, root)
	metadata := filepath.Join(root, "shifted.ffmetadata")
	if err := os.WriteFile(metadata, []byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=2000\nEND=4000\ntitle=Opening\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=4000\nEND=8000\ntitle=Ending\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "offset-chapters.mkv")
	args := []string{"-v", "error", "-copyts", "-itsoffset", "2", "-i", base, "-f", "ffmetadata", "-i", metadata, "-map", "0:v", "-map", "0:a", "-map_chapters", "1", "-c", "copy", "-avoid_negative_ts", "disabled", source}
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	info, err := Probe(context.Background(), probe, source)
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := strconv.ParseFloat(info.Format.StartTime, 64)
	if origin < 1.9 {
		t.Fatal("fixture lost its origin")
	}
	document := compositionTestDocument(t, worker, root, 16.0/9)
	for _, sample := range []bool{false, true} {
		target := filepath.Join(root, fmt.Sprintf("chapters-%t.mkv", sample))
		_, err := Compose(context.Background(), ffmpeg, probe, root, ComposeOptions{Source: source, Destination: target, Mode: "hard", Quality: "lossless", Sample: sample}, document, func(ComposeProgress) {})
		if err != nil {
			t.Fatal(err)
		}
		output, err := Probe(context.Background(), probe, target)
		if err != nil || len(output.Chapters) != 2 {
			t.Fatal(output.Chapters, err)
		}
		for i, chapter := range output.Chapters {
			start, _ := strconv.ParseFloat(chapter.StartTime, 64)
			end, _ := strconv.ParseFloat(chapter.EndTime, 64)
			wantEnd := []float64{4 - origin, 8 - origin}[i]
			if sample {
				wantEnd = min(wantEnd, 5)
			}
			if math.Abs(start-([]float64{2 - origin, 4 - origin}[i])) > .001 || math.Abs(end-wantEnd) > .001 || chapter.Tags["title"] != []string{"Opening", "Ending"}[i] {
				t.Fatal("shifted chapter content changed", chapter)
			}
		}
	}
	// MOV forces its first chapter start to zero. If normalization left
	// a real pre-chapter gap, reject that MP4 rather than silently erase it.
	target := filepath.Join(t.TempDir(), "chapter-gap.mp4")
	_, err = Compose(context.Background(), ffmpeg, probe, root, ComposeOptions{Source: source, Destination: target, Mode: "hard", Quality: "lossless"}, document, func(ComposeProgress) {})
	if err == nil {
		t.Fatal("MP4 changed the first chapter start without detection")
	}
	assertOnlyFiles(t, filepath.Dir(target))
}

// Burn-in must use the frozen ASS time, not the source's raw PTS. Compare
// lossless frame hashes, allowing only the interval where our cue is visible.
func TestNonzeroStartBurnTiming(t *testing.T) {
	worker, ffmpeg, probe := compositionTestTools(t)
	root := t.TempDir()
	source := filepath.Join(root, "offset.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30:duration=3", "-c:v", "libx264", "-output_ts_offset", "2", source).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	document := compositionTestDocument(t, worker, root, 16.0/9)
	target := filepath.Join(root, "burn.mp4")
	_, err := Compose(context.Background(), ffmpeg, probe, root, ComposeOptions{Source: source, Destination: target, Mode: "hard", Quality: "lossless"}, document, func(ComposeProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	hashes := func(path string) []string {
		raw, err := exec.Command(ffmpeg, "-v", "error", "-i", path, "-map", "0:V:0", "-pix_fmt", "yuv420p", "-fps_mode", "passthrough", "-f", "framemd5", "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		var result []string
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "0,") {
				fields := strings.Split(line, ",")
				result = append(result, strings.TrimSpace(fields[len(fields)-1]))
			}
		}
		return result
	}
	before, after := hashes(source), hashes(target)
	if len(before) != 90 || len(after) != len(before) {
		t.Fatal("frame count changed", len(before), len(after))
	}
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			if i < 3 || i >= 36 {
				t.Fatalf("subtitle burned outside 0.1–1.2 seconds at frame %d", i)
			}
			changed++
		}
	}
	if changed < 30 {
		t.Fatal("subtitle not burned on the expected frames", changed)
	}
}
