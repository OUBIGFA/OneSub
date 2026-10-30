package media

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// A self-authored 30 fps FFmetadata fixture makes FFmpeg produce the same
// QuickTime chapter carrier as ordinary downloaded MP4s (data/bin_data/text).
func chapterMediaFixture(t *testing.T, ffmpeg, root string) string {
	t.Helper()
	metadata := ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1400\ntitle=Opening\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=1400\nEND=5400\ntitle=Practice\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=5400\nEND=6000\ntitle=Ending\n"
	path := filepath.Join(root, "chapters.ffmetadata")
	if err := os.WriteFile(path, []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "chapter-source.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30:duration=6", "-f", "lavfi", "-i", "sine=frequency=440:duration=6", "-f", "ffmetadata", "-i", path, "-map", "0:v", "-map", "1:a", "-map", "1:a", "-map_chapters", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=jpn", source}
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("chapter fixture: %v %s", err, out)
	}
	return source
}

func compositionTestTools(t *testing.T) (string, string, string) {
	t.Helper()
	worker := os.Getenv("ONESUB_STYLE_WORKER")
	if worker == "" {
		t.Skip("build/test.ps1 enables the real style worker and FFmpeg")
	}
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Fatal("missing media tools")
	}
	return worker, ffmpeg, probe
}

func compositionTestDocument(t *testing.T, worker, root string, aspect float64) styles.Document {
	t.Helper()
	client := styles.Client{Executable: worker, Scratch: root}
	catalog, err := client.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document, err := client.Build(context.Background(), []subtitle.Cue{{ID: "sample", Text: "Composition test", StartMs: 100, EndMs: 1200}}, catalog.DefaultStyle, aspect)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

// Read raw ffprobe chapter values independently of Info, so omitting title
// or timing fields in the production parser cannot make preservation pass.
func assertChapterOutput(t *testing.T, probe, path string, sample bool) {
	t.Helper()
	raw, err := exec.Command(probe, "-v", "error", "-show_chapters", "-of", "json", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Chapters []struct {
			Start string `json:"start_time"`
			End   string `json:"end_time"`
			Tags  map[string]string
		}
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	titles := []string{"Opening", "Practice", "Ending"}
	starts, ends := []string{"0.000000", "1.400000", "5.400000"}, []string{"1.400000", "5.400000", "6.000000"}
	if sample {
		titles, starts, ends = titles[:2], starts[:2], []string{"1.400000", "5.000000"}
	}
	if len(got.Chapters) != len(titles) {
		t.Fatalf("wrong chapter count: %s", raw)
	}
	for i, chapter := range got.Chapters {
		if chapter.Start != starts[i] || chapter.End != ends[i] || chapter.Tags["title"] != titles[i] {
			t.Fatalf("chapter %d changed: %s", i, raw)
		}
	}
}

func TestRealCompositionPreservesChapterData(t *testing.T) {
	worker, ffmpeg, probe := compositionTestTools(t)
	root := t.TempDir()
	source := chapterMediaFixture(t, ffmpeg, root)
	input, err := Probe(context.Background(), probe, source)
	if err != nil {
		t.Fatal(err)
	}
	data := 0
	for _, s := range input.Streams {
		if s.CodecType == "data" && s.CodecName == "bin_data" {
			data++
		}
	}
	if data != 1 || len(input.Chapters) != 3 {
		t.Fatal("fixture did not produce a chapter data stream", input)
	}
	document := compositionTestDocument(t, worker, root, input.FrameAspect())
	for _, mode := range []string{"hard", "soft"} {
		for _, extension := range []string{".mp4", ".mkv"} {
			for _, sample := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s%s/sample%t", mode, extension, sample), func(t *testing.T) {
					output := filepath.Join(root, fmt.Sprintf("%s-%t%s", mode, sample, extension))
					options := ComposeOptions{Source: source, Destination: output, Mode: mode, Quality: "lossless", Sample: sample, Language: "en"}
					report, err := Compose(context.Background(), ffmpeg, probe, root, options, document, func(ComposeProgress) {})
					if err != nil {
						t.Fatal("chapter-bearing video rejected", err)
					}
					if report.Streams["video"] != 1 || report.Streams["audio"] != 2 || report.OutputSHA256 == "" {
						t.Fatal("lost audiovisual streams or validation report", report)
					}
					if mode == "soft" && report.Streams["subtitle"] != 1 {
						t.Fatal("soft subtitle missing", report)
					}
					assertChapterOutput(t, probe, output, sample)
					if mode == "hard" {
						// At three seconds the test subtitle is absent. Lossless
						// encoding must preserve the decoded frame, not just QP args.
						frameHash := func(path string) string {
							out, err := exec.Command(ffmpeg, "-v", "error", "-ss", "3", "-i", path, "-map", "0:v:0", "-frames:v", "1", "-pix_fmt", "yuv420p", "-f", "hash", "-hash", "sha256", "-").CombinedOutput()
							if err != nil {
								t.Fatalf("decode frame: %v %s", err, out)
							}
							return strings.TrimSpace(string(out))
						}
						if frameHash(source) != frameHash(output) {
							t.Fatal("lossless chapter handling changed a subtitle-free frame")
						}
					}
				})
			}
		}
	}
}

func TestChapterMappingRejectsUnidentifiedData(t *testing.T) {
	for _, extension := range []string{".mp4", ".mkv"} {
		for _, scenario := range []struct {
			name   string
			change func(*Info)
			allow  bool
		}{
			{"chapter-text", func(*Info) {}, true},
			{"chapter-tx3g", func(i *Info) { i.Streams[3].CodecTag = "tx3g" }, true},
			{"missing-chapters", func(i *Info) { i.Chapters = nil }, false},
			{"other-container", func(i *Info) { i.Format.FormatName = "mpegts" }, false},
			{"unknown-codec", func(i *Info) { i.Streams[3].CodecName = "unknown" }, false},
			{"timecode", func(i *Info) { i.Streams[3].CodecTag = "tmcd" }, false},
			{"camera-metadata", func(i *Info) { i.Streams[3].CodecTag = "gpmd" }, false},
			{"unknown-tag", func(i *Info) { i.Streams[3].CodecTag = "" }, false},
			{"extra-data", func(i *Info) {
				i.Streams = append(i.Streams, Stream{Index: 4, CodecType: "data", CodecName: "smpte_klv"})
			}, false},
		} {
			t.Run(scenario.name+extension, func(t *testing.T) {
				var info Info
				info.Format.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"
				info.Streams = []Stream{{Index: 0, CodecType: "video", CodecName: "av1", PixFmt: "yuv420p"}, {Index: 1, CodecType: "audio", CodecName: "aac"}, {Index: 2, CodecType: "subtitle", CodecName: "mov_text", CodecTag: "tx3g"}, {Index: 3, CodecType: "data", CodecName: "bin_data", CodecTag: "text", Tags: map[string]string{"handler_name": "SubtitleHandler"}}}
				info.Chapters = []Chapter{{StartTime: "0", EndTime: "6", Tags: map[string]string{"title": "Chapter"}}}
				scenario.change(&info)
				opts := ComposeOptions{Source: "source.mp4", Destination: "output" + extension, Mode: "hard", Quality: "lossless"}
				args, err := ComposeArgs(opts, info, "render.ass", "partial"+extension)
				if (err == nil) != scenario.allow {
					t.Fatal("chapter exception accepted or rejected the wrong data", err)
				}
				if scenario.allow {
					joined := strings.Join(args, " ")
					for _, mapping := range []string{"-map 0:0", "-map 0:1", "-map 0:2", "-map_chapters 0", "-qp 0"} {
						if !strings.Contains(joined, mapping) {
							t.Fatal("audiovisual/subtitle stream or chapters lost", joined)
						}
					}
					if strings.Contains(joined, "-map 0:3") {
						t.Fatal("chapter carrier was copied as a raw data stream", joined)
					}
				}
			})
		}
	}
}

func TestChapterValidationChecksTitlesAndTimes(t *testing.T) {
	source := []Chapter{{ID: 0, StartTime: "0", EndTime: "6", Tags: map[string]string{"title": "Chapter"}}}
	for _, scenario := range []struct {
		name         string
		output       []Chapter
		sample, want bool
	}{
		{"renumbered", []Chapter{{ID: 1, StartTime: "0.000000", EndTime: "6.000000", Tags: map[string]string{"title": "Chapter"}}}, false, true},
		{"millisecond-rounding", []Chapter{{StartTime: "0.0005", EndTime: "6.0005", Tags: map[string]string{"title": "Chapter"}}}, false, true},
		{"renamed", []Chapter{{StartTime: "0", EndTime: "6", Tags: map[string]string{"title": "Different"}}}, false, false},
		{"shifted", []Chapter{{StartTime: "0.1", EndTime: "6.1", Tags: map[string]string{"title": "Chapter"}}}, false, false},
		{"missing", nil, false, false},
		{"invalid", []Chapter{{StartTime: "NaN", EndTime: "6", Tags: map[string]string{"title": "Chapter"}}}, false, false},
		{"sample-clipped", []Chapter{{StartTime: "0", EndTime: "5", Tags: map[string]string{"title": "Chapter"}}}, true, true},
		{"sample-not-clipped", source, true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if got := chaptersPreserved(source, scenario.output, 0, scenario.sample); got != scenario.want {
				t.Fatal("incorrect chapter preservation result", got, scenario.want)
			}
		})
	}
}

// Local user media is opt-in and never becomes a repository fixture. This
// exercises the same five-second lossless path as the application's preview.
func TestLocalChapterMediaLosslessSample(t *testing.T) {
	source := os.Getenv("ONESUB_COMPOSE_TEST_SOURCE")
	if source == "" {
		t.Skip("set ONESUB_COMPOSE_TEST_SOURCE to validate a local media sample")
	}
	worker, ffmpeg, probe := compositionTestTools(t)
	root := t.TempDir()
	info, err := Probe(context.Background(), probe, source)
	if err != nil {
		t.Fatal(err)
	}
	document := compositionTestDocument(t, worker, root, info.FrameAspect())
	opts := ComposeOptions{Source: source, Destination: filepath.Join(root, "sample.mp4"), Mode: "hard", Quality: "lossless", Sample: true, Language: "en"}
	report, err := Compose(context.Background(), ffmpeg, probe, root, opts, document, func(ComposeProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	expectedVideo := streamCounts(info.Streams)["video"]
	if report.Streams["video"] != expectedVideo || report.Streams["audio"] < 1 || math.Abs(float64(report.DurationMs-5000)) > 250 || report.OutputSHA256 == "" {
		t.Fatal("local sample did not retain audiovisual content and validation", report.Streams, report.DurationMs)
	}
}
