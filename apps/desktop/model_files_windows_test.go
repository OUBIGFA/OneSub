//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// Restore only entries whose original directory and filename match this
// test-owned model. No unrelated recycle-bin item is touched or reported.
func restoreRecycledModel(t *testing.T, model models.Model, dir string) {
	t.Helper()
	names := []string{}
	for _, file := range model.Files {
		names = append(names, file.Name)
	}
	raw, _ := json.Marshal(names)
	command := jobs.CommandContext(context.Background(), "powershell.exe", "-NoProfile", "-Command", `
$ErrorActionPreference='Stop'
$wanted=$env:ONESUB_RECYCLE_TEST_NAMES | ConvertFrom-Json
$shell=New-Object -ComObject Shell.Application
$bin=$shell.Namespace(10)
$restored=0
foreach($item in @($bin.Items())) {
 $from=[string]$item.ExtendedProperty('System.Recycle.DeletedFrom')
 if(-not [string]::Equals($from,$env:ONESUB_RECYCLE_TEST_DIR,[StringComparison]::OrdinalIgnoreCase)){continue}
 $name=[string]$item.Name
 if($wanted -notcontains $name){continue}
 $item.InvokeVerb('undelete')
 $restored++
}
if($restored -ne $wanted.Count){throw "Expected $($wanted.Count) owned model entries, restored $restored"}
`)
	command.Env = append(os.Environ(), "ONESUB_RECYCLE_TEST_DIR="+dir, "ONESUB_RECYCLE_TEST_NAMES="+string(raw))
	if err := jobs.Run(context.Background(), command, nil); err != nil {
		t.Fatal("could not restore the test model from the real recycle bin", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for !model.FilesPresent(dir) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !model.FilesPresent(dir) {
		t.Fatal("restored files did not regain their original model paths")
	}
}

func recycleAndRestoreModel(t *testing.T, model models.Model, dir string) {
	t.Helper()
	if err := models.Verify(context.Background(), model, dir); err != nil {
		t.Fatal("test model was invalid before recycling", err)
	}
	extra := filepath.Join(dir, "unrelated-user-note.txt")
	if err := os.WriteFile(extra, []byte("keep unrelated content"), 0600); err != nil {
		t.Fatal(err)
	}
	recipe, _ := json.Marshal(modelRecycleRecipe{Model: model, Dir: dir})
	path := filepath.Join(t.TempDir(), "recipe.json")
	os.WriteFile(path, recipe, 0600)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := jobs.Run(ctx, jobs.CommandContext(ctx, exe, modelRecycleTestArgument, path), nil); err != nil {
		t.Fatal("real MyGo recycle operation failed", err)
	}
	if model.FilesPresent(dir) {
		t.Fatal("recycle operation left the model usable at its old path")
	}
	if text, err := os.ReadFile(extra); err != nil || string(text) != "keep unrelated content" {
		t.Fatal("recycling model assets changed unrelated files", err)
	}
	restoreRecycledModel(t, model, dir)
	if err := models.Verify(context.Background(), model, dir); err != nil {
		t.Fatal("restored model differs from the original bytes", err)
	}
}

// Tiny real files cover native recycle/restore even without downloaded ASR
// weights. Directory, content and hashes survive the Windows restore operation.
func TestModelFilesRecycleAndRestoreNative(t *testing.T) {
	dir := t.TempDir()
	model := models.Model{ID: "native-recycle-test", Kind: "asr"}
	for name, content := range map[string]string{"encoder.onnx": "graph", "encoder.weights": "weights"} {
		data := []byte(content)
		hash := sha256.Sum256(data)
		model.Files = append(model.Files, models.File{Name: name, Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])})
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	recycleAndRestoreModel(t, model, dir)
}

// Clone only test-owned weights, recycle them through MyGo, restore them via
// Windows, then discover and run the real SenseVoice engine at the same path.
func TestRestoredModelIsDiscoveredAndRecognizesRealSpeech(t *testing.T) {
	root := os.Getenv("ONESUB_SHERPA_TEST_MODELS")
	if root == "" {
		t.Skip("set ONESUB_SHERPA_TEST_MODELS for restored-model inference")
	}
	model, err := models.ByID(models.FallbackModel)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, file := range model.Files {
		input, err := os.Open(filepath.Join(root, model.ID, file.Name))
		if err != nil {
			t.Fatal(err)
		}
		output, err := os.Create(filepath.Join(dir, file.Name))
		if err != nil {
			input.Close()
			t.Fatal(err)
		}
		_, err = io.Copy(output, input)
		input.Close()
		closeErr := output.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
	}
	recycleAndRestoreModel(t, model, dir)
	a := nativeForTest(t)
	a.settings.AsrModel, a.settings.RecognitionMode = model.ID, recognitionManual
	a.settings.ModelPaths[model.ID] = dir
	a.refreshDownloadedAsrModels()
	if !a.selectedAsrDownloaded() {
		t.Fatal("restored model did not return to recognition choices")
	}
	a.asrModelChanged()
	if a.settings.Language != languageAuto || a.settings.ModelPaths[model.ID] != dir {
		t.Fatal("restoring changed the model folder or its detection default")
	}
	work := t.TempDir()
	tools := toolset{dataRoot: work, ffmpeg: os.Getenv("ONESUB_FFMPEG"), worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: work}}
	prefs := settings.Default()
	prefs.RecognitionMode, prefs.AsrModel, prefs.Language = recognitionManual, model.ID, languageAuto
	prefs.ModelPaths[model.ID] = dir
	srt, _, remove, err := tools.recognize(context.Background(), prefs, filepath.Join(root, "test_wavs", "zh.wav"), func(recognitionStep) {})
	if err != nil {
		t.Fatal("restored model could not recognize speech", err)
	}
	defer remove()
	doc, err := tools.worker.Read(context.Background(), srt)
	if err != nil || len(doc.Cues) == 0 {
		t.Fatal("restored model produced no subtitles", doc, err)
	}
}
