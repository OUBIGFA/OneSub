package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Translation uses the same streamed engine and real subtitle/checker
// workers as the desktop. A second MCP call reuses accepted results but
// still publishes a new file rather than overwriting either input or output.
func TestMCPTranslationSavesAndResumes(t *testing.T) {
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body struct {
			Stream   bool
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !body.Stream || len(body.Messages) != 2 {
			t.Error("invalid translation request", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/v1/chat/completions" || !strings.Contains(body.Messages[0].Content, "# Operation: translate\n") {
			t.Error("unexpected operation", r.URL.Path)
		}
		var payload struct{ Units []struct{ ID, Text string } }
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &payload); err != nil || len(payload.Units) != 1 || payload.Units[0].Text != "Hello, world" {
			t.Error("source did not reach the provider", err)
			http.Error(w, "bad units", http.StatusBadRequest)
			return
		}
		answer, _ := json.Marshal(map[string]string{payload.Units[0].ID: "你好，世界"})
		chunk, _ := json.Marshal(string(answer))
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", chunk)
	}))
	defer provider.Close()
	data := t.TempDir()
	prefs := settings.Default()
	prefs.ProviderURL, prefs.ProviderModel, prefs.OptimizeTimeline = provider.URL+"/v1", "protocol-test", false
	if err := settings.Save(filepath.Join(data, "settings.json"), prefs); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(data, "lesson.srt")
	const source = "1\n00:00:00,100 --> 00:00:02,900\nHello, world\n"
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	server, err := newMCPServer(t.TempDir(), data, os.Getenv("ONESUB_FFMPEG"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	serverSide, clientSide := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, name := range []string{"lesson-zh.srt", "lesson-zh-OneSub.srt"} {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "translate_subtitles", Arguments: map[string]any{"subtitle_path": input}})
		if err != nil || res.IsError {
			t.Fatal(res, err)
		}
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out subtitleOut
		if err := json.Unmarshal(raw, &out); err != nil || out.Cues != 1 || out.SubtitlePath != filepath.Join(data, name) || len(out.Warnings) != 0 {
			t.Fatal(string(raw), err)
		}
		if bytes, err := os.ReadFile(out.SubtitlePath); err != nil || !strings.Contains(string(bytes), "你好，世界") {
			t.Fatal("translated file not published", err)
		}
	}
	if requests.Load() != 1 {
		t.Fatal("accepted translation requested again", requests.Load())
	}
	if bytes, err := os.ReadFile(input); err != nil || string(bytes) != source {
		t.Fatal("source changed", err)
	}
}

// An MCP client, the official SDK's, sees OneSub's tools with their
// schemas and calls them; a bad call is a tool error, not a crash.
func TestMCPServerTools(t *testing.T) {
	data := t.TempDir()
	server, err := newMCPServer(t.TempDir(), data, os.Getenv("ONESUB_FFMPEG"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	serverSide, clientSide := mcp.NewInMemoryTransports()
	if _, err = server.Connect(ctx, serverSide, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.InitializeResult().ServerInfo.Name != "onesub" {
		t.Fatal(session.InitializeResult().ServerInfo)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.InputSchema == nil || tool.OutputSchema == nil || tool.Annotations == nil {
			t.Fatal("tool without schemas or annotations", tool.Name)
		}
	}
	if got := strings.Join(names, " "); got != "list_languages transcribe_media translate_subtitles" {
		t.Fatal(got)
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_languages"})
	if err != nil || res.IsError {
		t.Fatal(res, err)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), `"id":"ja"`) {
		t.Fatal(string(raw))
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "transcribe_media", Arguments: map[string]any{"media_path": "relative.mp4"}})
	if err != nil || !res.IsError {
		t.Fatal("a relative path was accepted", res, err)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "translate_subtitles", Arguments: map[string]any{"subtitle_path": filepath.Join(data, "a.srt")}})
	if err != nil || !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "translation service") {
		t.Fatal("translation without a service was not refused", res, err)
	}
	if !strings.Contains(mcpConfig(), `"args": [`) || !strings.Contains(mcpConfig(), `"mcp"`) {
		t.Fatal(mcpConfig())
	}
}
