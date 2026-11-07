package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A real SDK client chains file tools without opening a desktop or configuring
// a remote provider. Every write publishes a separate, readable artifact.
func TestMCPFileWorkflow(t *testing.T) {
	data := t.TempDir()
	server, err := newMCPServer(t.TempDir(), data, os.Getenv("ONESUB_FFMPEG"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var mu sync.Mutex
	lastProgress := map[string]float64{}
	var cancelComposition context.CancelFunc
	client := mcp.NewClient(&mcp.Implementation{Name: "workflow-test", Version: "1"}, &mcp.ClientOptions{ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
		mu.Lock()
		defer mu.Unlock()
		token := req.Params.ProgressToken.(string)
		if req.Params.Progress <= lastProgress[token] {
			t.Error("non-increasing MCP progress")
		}
		lastProgress[token] = req.Params.Progress
		if token == "cancel" && req.Params.Message == composeStageLabel(media.StageEncode) && cancelComposition != nil {
			cancelComposition()
		}
	}})
	var transport mcp.Transport
	if executable := os.Getenv("ONESUB_MCP_TEST_EXE"); executable != "" {
		// Final-package acceptance deliberately excludes development worker
		// overrides: all dependencies must resolve inside the shipped package.
		cmd := jobs.CommandContext(ctx, executable, "mcp")
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(strings.ToUpper(entry), "ONESUB_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "ONESUB_DATA_ROOT="+data)
		transport = &mcp.CommandTransport{Command: cmd}
	} else {
		st, ct := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		transport = ct
	}
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any, fails bool) map[string]any {
		t.Helper()
		params := &mcp.CallToolParams{Name: name, Arguments: args}
		if name == "compose_video" {
			params.SetProgressToken(args["mode"])
		}
		res, err := cs.CallTool(ctx, params)
		if err != nil || res.IsError != fails {
			t.Fatalf("%s: %v %v", name, res, err)
		}
		out := map[string]any{}
		b, _ := json.Marshal(res.StructuredContent)
		json.Unmarshal(b, &out)
		return out
	}
	input := filepath.Join(data, "source.srt")
	const source = "1\n00:00:00,100 --> 00:00:02,000\nHello world\n\n2\n00:00:02,100 --> 00:00:04,000\nSecond line\n"
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	page := call("read_subtitles", map[string]any{"subtitle_path": input, "offset": 1, "limit": 1}, false)
	if page["total"] != float64(2) || len(page["cues"].([]any)) != 1 {
		t.Fatal("paging lost cues", page)
	}
	output := filepath.Join(data, "edited.srt")
	args := map[string]any{"subtitle_path": input, "output_path": output, "expected_sha256": page["sha256"], "edits": []any{map[string]any{"index": 1, "text": "Updated line", "start_ms": 2200, "end_ms": 4200}}}
	call("edit_subtitles", args, false)
	result := call("read_subtitles", map[string]any{"subtitle_path": output}, false)
	second := result["cues"].([]any)[1].(map[string]any)
	if second["text"] != "Updated line" || second["start_ms"] != float64(2200) {
		t.Fatal("edits were not published", result)
	}
	call("edit_subtitles", args, true)
	args["output_path"] = filepath.Join(data, "stale.srt")
	args["expected_sha256"] = "stale"
	call("edit_subtitles", args, true)
	args["expected_sha256"] = page["sha256"]
	args["edits"] = []any{map[string]any{"index": 0, "text": "valid first edit"}, map[string]any{"index": 1, "end_ms": 1}}
	call("edit_subtitles", args, true)
	if _, err := os.Stat(args["output_path"].(string)); !os.IsNotExist(err) {
		t.Fatal("invalid batch published a partial result", err)
	}
	call("read_subtitles", map[string]any{"subtitle_path": input, "limit": -1}, true)
	call("read_subtitles", map[string]any{"subtitle_path": input, "limit": 501}, true)
	converted := filepath.Join(data, "edited.vtt")
	call("convert_subtitles", map[string]any{"subtitle_path": output, "output_path": converted}, false)
	call("check_subtitles", map[string]any{"subtitle_path": converted}, false)
	call("read_subtitles", map[string]any{"subtitle_path": converted}, false)
	for _, ext := range []string{"ass", "txt"} {
		call("convert_subtitles", map[string]any{"subtitle_path": output, "output_path": filepath.Join(data, "edited."+ext)}, false)
	}
	ass := filepath.Join(data, "edited.ass")
	assPage := call("read_subtitles", map[string]any{"subtitle_path": ass}, false)
	if assPage["cues"].([]any)[0].(map[string]any)["time_locked"] != true {
		t.Fatal("imported ASS timing restriction was not exposed")
	}
	call("edit_subtitles", map[string]any{"subtitle_path": ass, "output_path": filepath.Join(data, "retimed.ass"), "expected_sha256": assPage["sha256"], "edits": []any{map[string]any{"index": 0, "start_ms": 50}}}, true)
	unordered := filepath.Join(data, "unordered.srt")
	if err := os.WriteFile(unordered, []byte("1\n00:00:03,000 --> 00:00:04,000\nLater\n\n2\n00:00:00,000 --> 00:00:01,000\nEarlier\n\n3\n00:00:03,500 --> 00:00:04,500\nOverlap\n"), 0600); err != nil {
		t.Fatal(err)
	}
	checked := call("check_subtitles", map[string]any{"subtitle_path": unordered}, false)
	overlaps := 0
	for _, item := range checked["findings"].([]any) {
		finding := item.(map[string]any)
		if finding["code"] == "overlap" {
			overlaps++
			if finding["index"] != float64(2) {
				t.Fatal("out-of-order cue was incorrectly reported as overlapping", finding)
			}
		}
	}
	if overlaps != 1 {
		t.Fatal("actual overlap was not reported", checked)
	}
	caps := call("get_capabilities", nil, false)
	if caps["subtitle_worker"] != true || caps["style_worker"] != true || caps["media_tools"] != true {
		t.Fatal("bundled workers are not discoverable", caps)
	}
	// The same agent can inspect media, compose either subtitle mode and
	// inspect the resulting streams, without access to the desktop state.
	video := filepath.Join(data, "source.mp4")
	if out, err := exec.Command(os.Getenv("ONESUB_FFMPEG"), "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10:duration=5", "-f", "lavfi", "-i", "sine=frequency=440:duration=5", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", video).CombinedOutput(); err != nil {
		t.Fatalf("media fixture: %v %s", err, out)
	}
	info := call("inspect_media", map[string]any{"media_path": video}, false)
	if info["duration_ms"].(float64) < 4900 {
		t.Fatal("duration not returned", info)
	}
	for _, mode := range []string{"soft", "hard"} {
		composed := filepath.Join(data, mode+".mkv")
		args := map[string]any{"media_path": video, "subtitle_path": output, "output_path": composed, "mode": mode, "quality": "crf23"}
		result := call("compose_video", args, false)
		report := result["report"].(map[string]any)
		streams := report["streams"].(map[string]any)
		if streams["audio"] != float64(1) || streams["video"] != float64(1) || report["outputSha256"] == "" {
			t.Fatal("composition verification missing", report)
		}
		if mode == "soft" && streams["subtitle"] != float64(1) {
			t.Fatal("soft subtitle missing", report)
		}
		call("inspect_media", map[string]any{"media_path": composed}, false)
		call("compose_video", args, true)
	}
	mu.Lock()
	if len(lastProgress) != 2 {
		t.Error("composition progress did not reach the client")
	}
	mu.Unlock()
	// Cancel when real encoding begins, then wait for worker scratch cleanup
	// before checking that no partial or final video was published.
	cancelCtx, stopComposition := context.WithCancel(ctx)
	defer stopComposition()
	mu.Lock()
	cancelComposition = stopComposition
	mu.Unlock()
	cancelOutput := filepath.Join(data, "cancelled.mkv")
	cancelParams := &mcp.CallToolParams{Name: "compose_video", Arguments: map[string]any{"media_path": video, "subtitle_path": output, "output_path": cancelOutput, "mode": "hard"}}
	cancelParams.SetProgressToken("cancel")
	cancelResult, cancelErr := cs.CallTool(cancelCtx, cancelParams)
	if cancelErr == nil && (cancelResult == nil || !cancelResult.IsError) {
		t.Fatal("cancelled composition reported success")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		working, _ := filepath.Glob(filepath.Join(data, "cache", "worker", "compose-*"))
		if len(working) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancelled composition left its worker running")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(cancelOutput); !os.IsNotExist(err) {
		t.Fatal("cancelled composition published output", err)
	}
	partial, err := filepath.Glob(filepath.Join(data, ".onesub-*.partial*"))
	if err != nil || len(partial) > 0 {
		t.Fatal("cancelled composition left partial videos", partial, err)
	}
	if b, err := os.ReadFile(input); err != nil || string(b) != source {
		t.Fatal("input changed", err)
	}
}

// Invalid destinations must not incur a paid request, even with a complete
// provider configuration. Recognition must reject invalid formats first too.
func TestMCPPreflightAvoidsExpensiveWork(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "unexpected request", 500) }))
	defer provider.Close()
	data := t.TempDir()
	prefs := settings.Default()
	prefs.ProviderURL, prefs.ProviderModel = provider.URL, "test"
	if err := settings.Save(filepath.Join(data, "settings.json"), prefs); err != nil {
		t.Fatal(err)
	}
	server, err := newMCPServer(t.TempDir(), data, os.Getenv("ONESUB_FFMPEG"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "preflight", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	input := filepath.Join(data, "source.srt")
	if err := os.WriteFile(input, []byte("1\n00:00:00,000 --> 00:00:03,000\nHello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{input, filepath.Join(data, "wrong.mp4"), filepath.Join(data, "missing", "out.srt")} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "translate_subtitles", Arguments: map[string]any{"subtitle_path": input, "output_path": output}})
		if err != nil || !result.IsError {
			t.Fatal("invalid output was accepted", result, err)
		}
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "transcribe_media", Arguments: map[string]any{"media_path": input, "format": "invalid"}})
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "format must") {
		t.Fatal("invalid format reached recognition", result, err)
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request incurred translation traffic")
	}
}
