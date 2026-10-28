package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func testConfig(t *testing.T, server *httptest.Server) Config {
	t.Helper()
	root := os.Getenv("ONESUB_SKILL_ROOT")
	if root == "" {
		t.Fatal("Run build/test.ps1: ONESUB_SKILL_ROOT is required")
	}
	return Config{Endpoint: server.URL + "/v1", Model: "protocol-test", Key: "not-a-live-key", TargetLanguage: "zh-CN", SkillRoot: root, Client: server.Client(), MaxRequests: 100}
}
func replyJSON(w http.ResponseWriter, content any) {
	raw, _ := json.Marshal(content)
	json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 8}})
}
func testDocument(count int) subtitle.Document {
	doc := subtitle.Document{Format: "srt"}
	for i := 0; i < count; i++ {
		doc.Cues = append(doc.Cues, subtitle.Cue{ID: fmt.Sprintf("c%d", i), Position: i, Text: fmt.Sprintf("Source %d", i), OriginalText: fmt.Sprintf("Source %d", i), StartMs: int64(i) * 2000, EndMs: int64(i)*2000 + 1800})
	}
	return doc
}
func TestTranslateBatchesLocksAndRuleManifest(t *testing.T) {
	doc := testDocument(63)
	doc.Cues[7].TextLocked = true
	doc.Cues[8].ReadOnly = true
	doc.Cues[8].Text = "DRAWING_NEVER_SEND"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer not-a-live-key" {
			t.Error("wrong provider route")
		}
		var request struct {
			Model    string
			Messages []message
			Stream   bool
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
			t.Error("untrusted input crossed roles")
		}
		if !strings.Contains(request.Messages[0].Content, "## references/style-zh.md") || !strings.Contains(request.Messages[0].Content, "4.5") {
			t.Error("incomplete rules")
		}
		if strings.Contains(request.Messages[1].Content, "DRAWING_NEVER_SEND") {
			t.Error("protected drawing sent")
		}
		var data struct{ Cues []line }
		json.Unmarshal([]byte(request.Messages[1].Content), &data)
		if len(data.Cues) > 30 || len(data.Cues) == 0 {
			t.Error("bad batch size")
		}
		for i := range data.Cues {
			if data.Cues[i].ID == "c7" {
				t.Error("locked cue translated")
			}
			data.Cues[i].Text = strings.Replace(data.Cues[i].Text, "Source", "译文", 1)
		}
		replyJSON(w, candidate{Cues: data.Cues, Uncertainties: []string{}})
	}))
	defer server.Close()
	progress := 0
	result, err := Translate(context.Background(), testConfig(t, server), doc, func(done, total int) {
		progress = done
		if total != 61 {
			t.Error(total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || result.Report.Requests != 3 || result.Report.Translated != 61 || progress != 61 {
		t.Fatal(result.Report, progress)
	}
	if len(result.Report.Skipped) != 2 || len(result.Report.Rules) != 11 || result.Report.InputTokens != 36 {
		t.Fatal(result.Report)
	}
	if result.Document.Cues[0].Text != "译文 0" || result.Document.Cues[7].Text != "Source 7" || doc.Cues[0].Text != "Source 0" {
		t.Fatal("input mutated or result missing")
	}
	for _, rule := range result.Report.Rules {
		if len(rule.SHA256) != 64 {
			t.Fatal(rule)
		}
	}
}
func TestTranslateRejectsIncompleteOrInjectedCandidates(t *testing.T) {
	for name, body := range map[string]any{
		"missing":    candidate{},
		"wrong-id":   candidate{Cues: []line{{"other", "译文"}}},
		"blank":      candidate{Cues: []line{{"c0", "  "}}},
		"extra-time": map[string]any{"cues": []any{map[string]any{"id": "c0", "text": "译文", "startMs": 20}}},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { replyJSON(w, body) }))
			defer server.Close()
			doc := testDocument(1)
			_, err := Translate(context.Background(), testConfig(t, server), doc, nil)
			if err == nil || doc.Cues[0].Text != "Source 0" {
				t.Fatal("bad candidate accepted", err)
			}
		})
	}
}
func TestTranslateHTTPFailuresNeverRetry(t *testing.T) {
	for _, code := range []int{401, 429, 503, 307} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Location", "/other")
				w.WriteHeader(code)
			}))
			defer server.Close()
			result, err := Translate(context.Background(), testConfig(t, server), testDocument(1), nil)
			if err == nil || requests != 1 || result.Report.Requests != 1 {
				t.Fatal(err, requests, result.Report)
			}
		})
	}
}
func TestTranslateTimeoutReportsUnknownOutcome(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		time.Sleep(150 * time.Millisecond)
		replyJSON(w, candidate{})
	}))
	defer server.Close()
	config := testConfig(t, server)
	config.Client = &http.Client{Timeout: 40 * time.Millisecond}
	_, err := Translate(context.Background(), config, testDocument(1), nil)
	if err == nil || !strings.Contains(err.Error(), "OUTCOME_UNKNOWN") || requests != 1 {
		t.Fatal(err, requests)
	}
}
func TestTranslateFullSourceContextAndRequestCap(t *testing.T) {
	doc := testDocument(8)
	for i := range doc.Cues {
		doc.Cues[i].Text = "Start " + strings.Repeat("word ", 1300)
	}
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []message }
		json.NewDecoder(r.Body).Decode(&request)
		var data struct {
			Operation string
			Source    []line
			Cues      []line
		}
		json.Unmarshal([]byte(request.Messages[1].Content), &data)
		switch data.Operation {
		case "summarize":
			for _, l := range data.Source {
				seen[l.ID] = true
			}
			replyJSON(w, overview{Summary: "Whole-window summary"})
		case "merge_summaries":
			replyJSON(w, overview{Summary: "Merged all windows"})
		case "translate":
			for i := range data.Cues {
				data.Cues[i].Text = "完整译文"
			}
			replyJSON(w, candidate{Cues: data.Cues})
		default:
			t.Error("unknown operation")
		}
	}))
	defer server.Close()
	config := testConfig(t, server)
	if _, err := Translate(context.Background(), config, doc, nil); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 8 {
		t.Fatal("later context missing", seen)
	}
	config.MaxRequests = 1
	result, err := Translate(context.Background(), config, doc, nil)
	if err == nil || err.Error() != locales.ErrRequestLimit() || result.Report.Requests != 1 {
		t.Fatal(err, result.Report)
	}
}
func TestTranslateAllLockedDoesNotSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") }))
	defer server.Close()
	doc := testDocument(1)
	doc.Cues[0].TextLocked = true
	_, err := Translate(context.Background(), testConfig(t, server), doc, nil)
	if err == nil {
		t.Fatal("no-op reported translated")
	}
}
func TestEndpointRejectsUnsafeRoutes(t *testing.T) {
	for _, raw := range []string{"http://example.com/v1", "https://user:pass@example.com/v1", "https://example.com/v1?key=secret", "https://example.com/#token", "file:///test"} {
		if _, err := endpoint(raw); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"http://localhost:123/v1", "http://127.0.0.1:123/v1", "https://example.com/v1"} {
		if _, err := endpoint(raw); err != nil {
			t.Fatal(err)
		}
	}
}
