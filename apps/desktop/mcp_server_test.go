package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
