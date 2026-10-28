package translator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestTimelineFirstThenTranslationAndChecker(t *testing.T) {
	doc := testDocument(3)
	doc.Cues[2].TextLocked = true
	operations := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []message }
		json.NewDecoder(r.Body).Decode(&request)
		var data struct {
			Operation string
			Atoms     []subtitle.Atom
			Cues      []line
		}
		json.Unmarshal([]byte(request.Messages[1].Content), &data)
		operations = append(operations, data.Operation)
		if data.Operation == "segment" {
			replyJSON(w, map[string]any{"segments": []subtitle.SourceRange{{FirstAtom: "c0", LastAtom: "c1", Reason: "修复悬空连接词"}}, "uncertainties": []string{}})
		} else {
			if len(data.Cues) != 1 || data.Cues[0].Text != "Source 0 Source 1" {
				t.Error(data)
			}
			data.Cues[0].Text = "第 0 项和第 1 项"
			replyJSON(w, candidate{Cues: data.Cues})
		}
	}))
	defer server.Close()
	config := testConfig(t, server)
	config.OptimizeTimeline = true
	result, err := Translate(context.Background(), config, doc, nil)
	if err != nil || strings.Join(operations, ",") != "segment,translate" || result.Report.TimelineChanges != 1 || len(result.Document.Cues) != 2 {
		t.Fatal(result, operations, err)
	}
	if err = subtitle.ValidateTranslation(doc, result.Document); err != nil {
		t.Fatal(err)
	}
	checker := Checker{Executable: os.Getenv("ONESUB_SKILL_WORKER"), SkillRoot: config.SkillRoot, Scratch: t.TempDir(), Subtitle: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}}
	findings, err := checker.Check(context.Background(), result.Source, result.Document, "zh-CN")
	if err != nil || len(findings.Errors) != 0 {
		t.Fatal(findings, err)
	}
}
func TestInvalidTimelineStopsBeforeTranslation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		replyJSON(w, map[string]any{"segments": []subtitle.SourceRange{{FirstAtom: "c0", LastAtom: "c0"}}, "uncertainties": []string{}})
	}))
	defer server.Close()
	config := testConfig(t, server)
	config.OptimizeTimeline = true
	if _, err := Translate(context.Background(), config, testDocument(2), nil); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestProfileHotReloadWithoutExecution(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "custom")
	os.Mkdir(dir, 0700)
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte("---\nname: 自定义\n---\n第一版规则"), 0600)
	p, err := loadProfile(root, "custom")
	if err != nil || !strings.Contains(p.Rules, "第一版") {
		t.Fatal(p, err)
	}
	os.WriteFile(path, []byte("---\nname: 自定义\n---\n第二版规则"), 0600)
	next, err := loadProfile(root, "custom")
	if err != nil || p.SHA256 == next.SHA256 {
		t.Fatal(next, err)
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
func TestRetranslationUsesSourceAndRetainsLockedProvenance(t *testing.T) {
	doc := testDocument(2)
	for i := range doc.Cues {
		doc.Cues[i].TranslationSource = doc.Cues[i].Text
		doc.Cues[i].Text = "旧译文"
		doc.Cues[i].Translated = true
		doc.Cues[i].SourceRange = &subtitle.SourceRange{FirstAtom: "old", LastAtom: "old"}
	}
	doc.Cues[1].TextLocked = true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []message }
		json.NewDecoder(r.Body).Decode(&request)
		var data struct{ Cues []line }
		json.Unmarshal([]byte(request.Messages[1].Content), &data)
		if data.Cues[0].Text != "Source 0" {
			t.Error(data)
		}
		data.Cues[0].Text = "新译文 0"
		replyJSON(w, candidate{Cues: data.Cues})
	}))
	defer server.Close()
	result, err := Translate(context.Background(), testConfig(t, server), doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = subtitle.ValidateTranslation(doc, result.Document); err != nil {
		t.Fatal(err)
	}
}
