package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// fakeModel is an OpenAI-compatible service for tests. It streams each
// answer in small Server-Sent Events chunks, as real services do, and
// records every request.
type fakeModel struct {
	t        *testing.T
	mu       sync.Mutex
	requests []seen
	// reply answers a request: the content, the finish reason, and an HTTP
	// status other than 200 when the request fails.
	reply func(r seen, n int) (content, finish string, status int)
}

type seen struct {
	Operation string // segment, translate, analyze or merge summaries
	System    string
	Messages  []message
	Format    string         // the response_format type sent, or ""
	Schema    map[string]any // its schema
	Payload   translatePayload
	Usage     bool // stream_options asked for usage
}

func (f *fakeModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model          string
		Messages       []message
		Stream         bool
		StreamOptions  *struct{ IncludeUsage bool } `json:"stream_options"`
		ResponseFormat *struct {
			Type       string
			JSONSchema struct{ Schema map[string]any } `json:"json_schema"`
		} `json:"response_format"`
	}
	if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer not-a-live-key" {
		f.t.Error("wrong provider route", r.URL.Path)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// A cancelled client may stop while sending its body. That is the
		// cancellation test's transport event, not an invalid model request.
		if r.Context().Err() == nil {
			f.t.Error("not a streamed chat request", err)
		}
		return
	}
	if !body.Stream || len(body.Messages) == 0 || body.Messages[0].Role != "system" {
		f.t.Error("not a streamed chat request")
		return
	}
	s := seen{System: body.Messages[0].Content, Messages: body.Messages[1:], Usage: body.StreamOptions != nil}
	if body.ResponseFormat != nil {
		s.Format, s.Schema = body.ResponseFormat.Type, body.ResponseFormat.JSONSchema.Schema
	}
	for _, op := range []string{"segment", "translate", "analyze", "merge summaries"} {
		if strings.Contains(s.System, "# Operation: "+op+"\n") {
			s.Operation = op
		}
	}
	if s.Operation == "translate" {
		json.Unmarshal([]byte(s.Messages[0].Content), &s.Payload)
	}
	f.mu.Lock()
	f.requests = append(f.requests, s)
	n := len(f.requests)
	f.mu.Unlock()
	content, finish, status := f.reply(s, n)
	if status != 0 && status != http.StatusOK {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(status)
		fmt.Fprint(w, content)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	runes := []rune(content)
	for start := 0; start < len(runes); start += 7 {
		chunk, _ := json.Marshal(string(runes[start:min(start+7, len(runes))]))
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s}}]}\n\n", chunk)
	}
	if finish != "" {
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":%q}]}\n\n", finish)
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":8}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func (f *fakeModel) seen() []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seen(nil), f.requests...)
}

// translated is the default answer: every unit's text, marked translated.
func translated(s seen) string {
	ids, values := []string{}, map[string]string{}
	for _, u := range s.Payload.Units {
		ids = append(ids, u.ID)
		values[u.ID] = strings.Replace(u.Text, "Source", "译文", 1)
	}
	return orderedObject(ids, values)
}

func serve(t *testing.T, reply func(s seen, n int) (string, string, int)) (*fakeModel, Config) {
	t.Helper()
	f := &fakeModel{t: t, reply: reply}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	root := os.Getenv("ONESUB_SKILL_ROOT")
	if root == "" {
		t.Fatal("Run build/test.ps1: ONESUB_SKILL_ROOT is required")
	}
	return f, Config{Endpoint: server.URL + "/v1", Model: "protocol-test", Key: "not-a-live-key", TargetLanguage: "zh-CN", SkillRoot: root, Client: server.Client(), MaxRequests: 100, CacheDir: t.TempDir()}
}

func ok(s seen, _ int) (string, string, int) { return translated(s), "stop", 0 }

func TestTranslationUsesConfiguredBatchSize(t *testing.T) {
	for _, test := range []struct {
		config string
		count  int
		sizes  []int
	}{
		{`{"Concurrency":1}`, 43, []int{20, 20, 3}},
		{`{"Concurrency":1,"BatchSize":4}`, 9, []int{4, 4, 1}},
	} {
		f, config := serve(t, ok)
		if err := json.Unmarshal([]byte(test.config), &config); err != nil {
			t.Fatal(err)
		}
		result, err := Translate(context.Background(), config, testDocument(test.count))
		if err != nil {
			t.Fatal(err)
		}
		sizes := []int{}
		for _, request := range f.seen() {
			sizes = append(sizes, len(request.Payload.Units))
		}
		if !slices.Equal(sizes, test.sizes) || result.Report.Translated != test.count {
			t.Fatal("wrong request batches", sizes, test.sizes, result.Report)
		}
	}
}

func TestConfiguredConcurrencyRunsThreeBatchesAtOnce(t *testing.T) {
	var mu sync.Mutex
	var once sync.Once
	active, peak := 0, 0
	gate := make(chan struct{})
	timer := time.AfterFunc(2*time.Second, func() { once.Do(func() { close(gate) }) })
	defer timer.Stop()
	f, config := serve(t, func(s seen, _ int) (string, string, int) {
		mu.Lock()
		active++
		peak = max(peak, active)
		if active == 3 {
			once.Do(func() { close(gate) })
		}
		mu.Unlock()
		<-gate
		mu.Lock()
		active--
		mu.Unlock()
		return translated(s), "stop", 0
	})
	config.Concurrency, config.BatchSize = 3, 2
	result, err := Translate(context.Background(), config, testDocument(9))
	if err != nil || peak != 3 || len(f.seen()) != 5 || result.Report.Translated != 9 {
		t.Fatal("configured concurrency was not used", err, peak, len(f.seen()), result.Report)
	}
}

// A failed later request must still return every accepted subtitle. The caller
// can show and persist this result while the error remains visible.
func TestInterruptedTranslationReturnsAcceptedSubtitles(t *testing.T) {
	f, config := serve(t, func(s seen, n int) (string, string, int) {
		if n > 1 {
			return "invalid key", "", http.StatusUnauthorized
		}
		return translated(s), "stop", 0
	})
	config.Concurrency = 1
	result, err := Translate(context.Background(), config, testDocument(35))
	if err == nil {
		t.Fatal("interruption reported success")
	}
	if len(result.Document.Cues) != 35 || !result.Document.Cues[0].Translated || result.Document.Cues[0].Text != "译文 0" || result.Document.Cues[34].Translated || result.Report.Translated != 20 {
		t.Fatal("accepted subtitles disappeared", result.Report, result.Document.Cues[0])
	}
	if len(f.seen()) != 2 {
		t.Fatal("unexpected retry", len(f.seen()))
	}
}

func TestCutStreamResumesOnlyMissingUnitsWithAnotherModel(t *testing.T) {
	f, config := serve(t, func(s seen, n int) (string, string, int) {
		if n == 1 {
			return `{"1":"完整译文","2":"未完成`, "", 0
		}
		return translated(s), "stop", 0
	})
	doc := testDocument(2)
	first, err := Translate(context.Background(), config, doc)
	if err == nil || first.Resume == nil || first.Resume.Completed[0] != "完整译文" || len(first.Resume.Completed) != 1 || len(f.seen()) != 1 {
		t.Fatal("cut stream was not kept", err, first.Resume)
	}
	config.Model, config.CacheDir, config.Resume = "another-model", t.TempDir(), first.Resume
	second, err := Translate(context.Background(), config, doc)
	requests := f.seen()
	if err != nil || len(requests) != 2 || len(requests[1].Payload.Units) != 1 || requests[1].Payload.Units[0].ID != "2" || second.Document.Cues[0].Text != "完整译文" || second.Report.Resumed != 1 {
		t.Fatal("resume repeated completed units", err, requests, second.Report)
	}
	doc.Cues[0].Text = "Changed source"
	if _, err = Translate(context.Background(), config, doc); err == nil || len(f.seen()) != 2 {
		t.Fatal("stale progress reused")
	}
}

func testDocument(count int) subtitle.Document {
	doc := subtitle.Document{Format: "srt"}
	for i := 0; i < count; i++ {
		doc.Cues = append(doc.Cues, subtitle.Cue{ID: fmt.Sprintf("c%d", i), Position: i, Text: fmt.Sprintf("Source %d", i), OriginalText: fmt.Sprintf("Source %d", i), StartMs: int64(i) * 2000, EndMs: int64(i)*2000 + 1800})
	}
	return doc
}

// Every batch carries the skill's rules for its phase, the whole source as
// read-only context and at most 20 subtitles; locked and protected
// subtitles keep their text, and drawings are never sent. The live view
// sees each subtitle while it is written and when it is accepted.
func TestTranslateStreamsEveryBatchWithItsRules(t *testing.T) {
	doc := testDocument(63)
	doc.Cues[7].TextLocked = true
	doc.Cues[8].ReadOnly = true
	doc.Cues[8].Text = "DRAWING_NEVER_SEND"
	f, config := serve(t, ok)
	var events []Event
	config.Observe = func(ev Event) { events = append(events, ev) }
	result, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	requests := f.seen()
	if len(requests) != 4 || result.Report.Requests != 4 || result.Report.Translated != 61 {
		t.Fatal(len(requests), result.Report)
	}
	for _, r := range requests {
		if r.Operation != "translate" || r.Format != "json_schema" || !r.Usage || len(r.Payload.Units) > 20 {
			t.Fatal("bad batch", r.Operation, r.Format, len(r.Payload.Units))
		}
		for _, file := range []string{"## SKILL.md", "## references/style-common.md", "## references/style-zh.md", "## references/names-zh.md", "## references/reading-load.md", `"max_cps":4.5`} {
			if !strings.Contains(r.System, file) {
				t.Fatal("rules missing", file)
			}
		}
		for _, file := range []string{"## references/glossary-3d-zh.md", "## references/segmentation.md", "## references/asd-ste100-subtitles.md"} {
			if strings.Contains(r.System, file) {
				t.Fatal("general style sent", file)
			}
		}
		if strings.Contains(r.Messages[0].Content, "DRAWING_NEVER_SEND") {
			t.Fatal("protected drawing sent")
		}
		context, _ := json.Marshal(r.Payload.Context)
		if !strings.Contains(string(context), "Source 62") {
			t.Fatal("whole source missing from the context")
		}
		required, _ := r.Schema["required"].([]any)
		if len(required) != len(r.Payload.Units) {
			t.Fatal("schema does not fix the batch's keys", r.Schema)
		}
	}
	if result.Document.Cues[0].Text != "译文 0" || result.Document.Cues[7].Text != "Source 7" || doc.Cues[0].Text != "Source 0" {
		t.Fatal("input mutated or result missing")
	}
	if err = subtitle.ValidateTranslation(doc, result.Document); err != nil {
		t.Fatal(err)
	}
	units, streamed, final := 0, false, map[int]bool{}
	for _, ev := range events {
		units = max(units, len(ev.Units))
		for _, u := range ev.Updates {
			streamed = streamed || !u.Final
			if u.Final {
				final[u.Index] = true
			}
		}
	}
	if units != 63 || !streamed || len(final) != 61 || events[len(events)-1].Done != 61 {
		t.Fatal("live view incomplete", units, streamed, len(final))
	}
}

// An answer that misses a subtitle or leaves one in the source language is
// answered by a request for just those, naming what was wrong.
func TestTranslateAsksAgainForWhatAnAnswerGotWrong(t *testing.T) {
	doc := testDocument(4)
	doc.Cues[2].Text = "Source text number two goes here"
	doc.Cues[2].OriginalText = doc.Cues[2].Text
	f, config := serve(t, func(s seen, n int) (string, string, int) {
		if n == 1 {
			return `{"1": "译文 0", "3": "Source text number two goes here", "4": "译文 3"}`, "stop", 0
		}
		return translated(s), "stop", 0
	})
	result, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	requests := f.seen()
	if len(requests) != 2 || len(requests[1].Payload.Units) != 2 || len(requests[1].Payload.PreviousProblems) != 2 {
		t.Fatal(requests[1].Payload)
	}
	if !strings.Contains(strings.Join(requests[1].Payload.PreviousProblems, "\n"), "unit 3 is still in the source language") {
		t.Fatal(requests[1].Payload.PreviousProblems)
	}
	if result.Document.Cues[2].Text != "译文 text number two goes here" {
		t.Fatal(result.Document.Cues[2].Text)
	}
}

// Subtitles a batch keeps missing are asked for one at a time, as
// SmartSub's repair does.
func TestTranslateRepairsOneSubtitleAtATime(t *testing.T) {
	f, config := serve(t, func(s seen, n int) (string, string, int) {
		if len(s.Payload.Units) > 1 {
			return `{"1": "译文 0"}`, "stop", 0
		}
		return translated(s), "stop", 0
	})
	result, err := Translate(context.Background(), config, testDocument(3))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(f.seen()); n != 4 || result.Document.Cues[2].Text != "译文 2" {
		t.Fatal(n, result.Document.Cues)
	}
}

// A run that cannot translate a subtitle fails, naming how many, and fills
// nothing with the source; the next run sends only what is missing.
func TestTranslateIncompleteRunResumesFromCache(t *testing.T) {
	doc := testDocument(3)
	failing := true
	f, config := serve(t, func(s seen, _ int) (string, string, int) {
		if failing {
			return strings.Replace(translated(s), `"2":"译文 1"`, `"2":""`, 1), "stop", 0
		}
		return translated(s), "stop", 0
	})
	_, err := Translate(context.Background(), config, doc)
	if err == nil || err.Error() != locales.ErrTranslateIncomplete(1) || len(f.seen()) != 3 {
		t.Fatal(err, len(f.seen()))
	}
	failing = false
	result, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	requests := f.seen()[3:]
	if len(requests) != 1 || len(requests[0].Payload.Units) != 1 || requests[0].Payload.Units[0].ID != "2" || result.Report.Resumed != 2 {
		t.Fatal(len(requests), result.Report)
	}
	if result.Document.Cues[0].Text != "译文 0" || result.Document.Cues[1].Text != "译文 1" {
		t.Fatal(result.Document.Cues)
	}
}

// Overloaded services and cut streams are sent again after a wait, and the
// live view says why.
func TestTranslateRetriesTransientFailures(t *testing.T) {
	f, config := serve(t, func(s seen, n int) (string, string, int) {
		switch n {
		case 1:
			return `{"error":{"message":"overloaded"}}`, "", http.StatusServiceUnavailable
		case 2:
			return `{"1": "译文`, "", 0 // the stream ends without a finish
		}
		return translated(s), "stop", 0
	})
	notes := []string{}
	config.Observe = func(ev Event) {
		if ev.Retry != "" {
			notes = append(notes, ev.Retry)
		}
	}
	result, err := Translate(context.Background(), config, testDocument(1))
	if err != nil || len(f.seen()) != 3 || result.Report.Requests != 3 || len(notes) != 2 || !strings.Contains(notes[0], "overloaded") {
		t.Fatal(err, len(f.seen()), notes)
	}
}

// Credentials, a missing model, a request the service cannot take and a
// redirect are not sent again.
func TestTranslatePermanentFailuresStop(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusBadRequest, http.StatusTemporaryRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f, config := serve(t, func(seen, int) (string, string, int) {
				return `{"error":{"message":"maximum context length exceeded"}}`, "", status
			})
			_, err := Translate(context.Background(), config, testDocument(1))
			// A 400 may first step down from the JSON schema; it never retries.
			if err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) || len(f.seen()) > 3 {
				t.Fatal(err, len(f.seen()))
			}
		})
	}
}

// A service that rejects JSON schemas is asked for JSON objects, and the
// step down holds for the rest of the run.
func TestStructuredOutputStepsDown(t *testing.T) {
	f, config := serve(t, func(s seen, _ int) (string, string, int) {
		if s.Format == "json_schema" {
			return `{"error":{"message":"response_format type json_schema is not supported"}}`, "", http.StatusBadRequest
		}
		return translated(s), "stop", 0
	})
	config.Concurrency = 1
	if _, err := Translate(context.Background(), config, testDocument(40)); err != nil {
		t.Fatal(err)
	}
	formats := []string{}
	for _, r := range f.seen() {
		formats = append(formats, r.Format)
	}
	if !slices.Equal(formats, []string{"json_schema", "json_object", "json_object"}) {
		t.Fatal(formats)
	}
}

// Reasoning, fences, trailing commas and nested values in an answer do not
// cost a batch.
func TestTranslateReadsLooseAnswers(t *testing.T) {
	_, config := serve(t, func(seen, int) (string, string, int) {
		return "<think>The user wants {json}.</think>\n```json\n{\"1\": \"译文一\",\n \"2\": {\"src\": \"Source 1\", \"tr\": \"译文二\"},}\n```", "stop", 0
	})
	result, err := Translate(context.Background(), config, testDocument(2))
	if err != nil || result.Document.Cues[0].Text != "译文一" || result.Document.Cues[1].Text != "译文二" {
		t.Fatal(err, result.Document.Cues)
	}
}

// An answer cut by the output limit keeps its complete subtitles; the rest
// are asked for again.
func TestTranslateKeepsWhatACutAnswerFinished(t *testing.T) {
	f, config := serve(t, func(s seen, n int) (string, string, int) {
		if n == 1 {
			return `{"1": "一", "2": "二", "3": "三`, "length", 0
		}
		return translated(s), "stop", 0
	})
	result, err := Translate(context.Background(), config, testDocument(3))
	requests := f.seen()
	if err != nil || len(requests) != 2 || len(requests[1].Payload.Units) != 1 || result.Document.Cues[2].Text != "译文 2" || result.Document.Cues[1].Text != "二" {
		t.Fatal(err, len(requests), result.Document.Cues)
	}
}

// The request limit caps a run, retries included.
func TestTranslateRequestLimit(t *testing.T) {
	f, config := serve(t, ok)
	config.MaxRequests, config.Concurrency = 1, 1
	result, err := Translate(context.Background(), config, testDocument(40))
	if err == nil || err.Error() != locales.ErrRequestLimit() || len(f.seen()) != 1 || result.Report.Requests != 1 {
		t.Fatal(err, result.Report)
	}
}

// A long source is read once into a summary and working glossary, from
// its beginning to its end, which every batch then carries.
func TestLongSourceIsReadIntoASummary(t *testing.T) {
	doc := testDocument(8)
	for i := range doc.Cues {
		doc.Cues[i].Text = fmt.Sprintf("Source %d ", i) + strings.Repeat("word ", 400)
	}
	f, config := serve(t, func(s seen, _ int) (string, string, int) {
		switch s.Operation {
		case "analyze":
			return `{"summary": "A lesson in eight parts", "terms": ["word → 词"]}`, "stop", 0
		case "translate":
			return translated(s), "stop", 0
		}
		return "", "stop", 0
	})
	if _, err := Translate(context.Background(), config, doc); err != nil {
		t.Fatal(err)
	}
	requests := f.seen()
	if requests[0].Operation != "analyze" || !strings.Contains(requests[0].Messages[0].Content, "Source 7") {
		t.Fatal("the summary did not read the whole source")
	}
	for _, r := range requests[1:] {
		context, _ := json.Marshal(r.Payload.Context)
		if r.Operation != "translate" || !strings.Contains(string(context), "A lesson in eight parts") || len(r.Payload.Neighbors) == 0 && len(r.Payload.Units) < 8 {
			t.Fatal("batch without the summary or its neighbors", string(context))
		}
	}
}

// A style adds the skill references it names, the 3D glossary only for a
// Chinese target; the core prompt is the same for every style.
func TestStylesAddTheirReferences(t *testing.T) {
	for _, c := range []struct {
		style, target string
		want, not     []string
	}{
		{"3d", "zh-CN", []string{"## references/glossary-3d-zh.md", "## references/asd-ste100-subtitles.md"}, nil},
		{"teaching", "zh-CN", []string{"## references/asd-ste100-subtitles.md"}, []string{"## references/glossary-3d-zh.md"}},
		{"general", "zh-CN", nil, []string{"## references/glossary-3d-zh.md", "## references/asd-ste100-subtitles.md"}},
		{"3d", "en", []string{"## references/asd-ste100-subtitles.md"}, []string{"## references/glossary-3d-zh.md", "## references/style-zh.md"}},
	} {
		f, config := serve(t, ok)
		config.ProfileID, config.TargetLanguage = c.style, c.target
		if _, err := Translate(context.Background(), config, testDocument(1)); err != nil {
			t.Fatal(err)
		}
		system := f.seen()[0].System
		for _, w := range c.want {
			if !strings.Contains(system, w) {
				t.Fatal(c.style, c.target, "missing", w)
			}
		}
		for _, n := range c.not {
			if strings.Contains(system, n) {
				t.Fatal(c.style, c.target, "unexpected", n)
			}
		}
	}
}

func TestTranslateAllLockedDoesNotSend(t *testing.T) {
	f, config := serve(t, ok)
	doc := testDocument(1)
	doc.Cues[0].TextLocked = true
	if _, err := Translate(context.Background(), config, doc); err == nil || len(f.seen()) != 0 {
		t.Fatal("no-op reported translated", err)
	}
}

func TestEndpointRejectsUnsafeRoutes(t *testing.T) {
	for _, raw := range []string{"http://example.com/v1", "https://user:pass@example.com/v1", "https://example.com/v1?key=secret", "https://example.com/#token", "file:///test"} {
		if _, err := endpoint(raw); err == nil {
			t.Fatal(raw)
		}
	}
	for raw, want := range map[string]string{"http://localhost:123/v1": "http://localhost:123/v1", "http://127.0.0.1:123/v1/": "http://127.0.0.1:123/v1", "https://example.com/v1/chat/completions": "https://example.com/v1"} {
		if got, err := endpoint(raw); err != nil || got != want {
			t.Fatal(raw, got, err)
		}
	}
}

// The Responses protocol streams its events, sends the rules as
// instructions, asks the service not to store the request, and treats an
// incomplete answer as cut.
func TestResponsesProtocolStreams(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request struct {
			Model, Instructions string
			Input               []message
			Store               *bool
			Stream              bool
			Text                struct{ Format struct{ Type string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || r.URL.Path != "/v1/responses" {
			t.Error(err, r.URL.Path)
		}
		if !strings.Contains(request.Instructions, "## SKILL.md") || request.Store == nil || *request.Store || !request.Stream || request.Text.Format.Type != "json_schema" {
			t.Error("not a Responses request")
		}
		var payload translatePayload
		json.Unmarshal([]byte(request.Input[0].Content), &payload)
		answer := translated(seen{Payload: payload})
		end := `{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5}}}`
		if requests == 1 {
			answer, end = `{"1": "译文 0"`, `{"type":"response.incomplete","response":{"usage":{"input_tokens":10,"output_tokens":5}}}`
		}
		w.Header().Set("Content-Type", "text/event-stream")
		delta, _ := json.Marshal(answer)
		fmt.Fprintf(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":%s}\n\n", delta)
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", end)
	}))
	defer server.Close()
	config := Config{Endpoint: server.URL + "/v1", API: APIResponses, Model: "protocol-test", TargetLanguage: "zh-CN", SkillRoot: os.Getenv("ONESUB_SKILL_ROOT"), Client: server.Client(), MaxRequests: 10}
	result, err := Translate(context.Background(), config, testDocument(2))
	if err != nil || requests != 2 || result.Document.Cues[1].Text != "译文 1" || result.Report.InputTokens != 20 {
		t.Fatal(err, requests, result.Report)
	}
}

func TestProfileHotReloadWithoutExecution(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "custom")
	os.Mkdir(dir, 0700)
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte("---\nname: 自定义\nreferences: references/glossary-3d-zh.md\n---\n第一版规则"), 0600)
	p, err := loadProfile(root, "custom")
	if err != nil || !strings.Contains(p.Rules, "第一版") || !slices.Equal(p.References, []string{"references/glossary-3d-zh.md"}) {
		t.Fatal(p, err)
	}
	os.WriteFile(path, []byte("---\nname: 自定义\n---\n第二版规则"), 0600)
	next, err := loadProfile(root, "custom")
	if err != nil || p.SHA256 == next.SHA256 {
		t.Fatal(next, err)
	}
	// A style names files of the skill, never a path out of it.
	os.WriteFile(path, []byte("---\nname: 自定义\nreferences: ../../secret.md\n---\n规则"), 0600)
	if _, err := loadProfile(root, "custom"); err == nil {
		t.Fatal("a path out of the skill accepted")
	}
	os.Remove(path)
	if _, err := loadProfile(root, "custom"); err == nil {
		t.Fatal("deleted style cached")
	}
	profiles, err := Profiles(root)
	if err != nil || len(profiles) != 8 {
		t.Fatal(profiles, err)
	}
}

// Translating again starts from the source, never the earlier translation,
// and keeps a locked hand edit with its provenance.
func TestRetranslationUsesSourceAndRetainsLockedProvenance(t *testing.T) {
	doc := testDocument(2)
	for i := range doc.Cues {
		doc.Cues[i].TranslationSource = doc.Cues[i].Text
		doc.Cues[i].Text = "旧译文"
		doc.Cues[i].Translated = true
		doc.Cues[i].SourceRange = &subtitle.SourceRange{FirstAtom: "old", LastAtom: "old"}
	}
	doc.Cues[1].TextLocked = true
	f, config := serve(t, ok)
	result, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	if units := f.seen()[0].Payload.Units; len(units) != 1 || units[0].Text != "Source 0" {
		t.Fatal(units)
	}
	if err = subtitle.ValidateTranslation(doc, result.Document); err != nil {
		t.Fatal(err)
	}
}

// Cancelling stops the run between requests.
func TestTranslateCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, config := serve(t, func(s seen, _ int) (string, string, int) {
		cancel()
		return translated(s), "stop", 0
	})
	// This case checks cancellation between batches, not midway through a
	// second concurrent request reaching the fixture.
	config.Concurrency = 1
	_, err := Translate(ctx, config, testDocument(40))
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestAdvertisedOutputLimitReachesBothProtocols(t *testing.T) {
	for _, api := range []string{APIChatCompletions, APIResponses} {
		t.Run(api, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Completion int `json:"max_completion_tokens"`
					Output     int `json:"max_output_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if api == APIResponses {
					if body.Output != 8192 || body.Completion != 0 {
						t.Error("wrong Responses output limit", body)
					}
					fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"1\\\":\\\"译文 0\\\"}\"}\n\n")
					fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{}}\n\n")
				} else {
					if body.Completion != 8192 || body.Output != 0 {
						t.Error("wrong Chat output limit", body)
					}
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"1\\\":\\\"译文 0\\\"}\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			config := Config{Endpoint: server.URL, API: api, Model: "limits-test", TargetLanguage: "zh-CN", SkillRoot: os.Getenv("ONESUB_SKILL_ROOT"), MaxRequests: 1, MaxOutputTokens: 8192, Client: server.Client()}
			result, err := Translate(context.Background(), config, testDocument(1))
			if err != nil || calls != 1 || !result.Document.Cues[0].Translated {
				t.Fatal("limit-bearing request failed", err, calls)
			}
		})
	}
}
