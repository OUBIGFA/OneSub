package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// A task freezes its input, language, style and output location. Retry may use
// the provider the user now selects, while retaining already completed units.
// Config's key, HTTP client, observer and resume pointer are never serialized.
type translationTask struct {
	ProjectID       string            `json:"projectId"`
	ProjectPath     string            `json:"projectPath"`
	Revision        int64             `json:"revision"`
	Source          subtitle.Document `json:"source"`
	Config          translator.Config `json:"config"`
	CredentialScope string            `json:"credentialScope"`
	ProviderName    string            `json:"providerName"`
	Target          string            `json:"target"`
	Format          string            `json:"format"`
	SavedHash       [2]string         `json:"savedHash"`
	SavedPath       [2]string         `json:"savedPath"`
}

// Small appended events keep every displayed fragment durable without
// rewriting a movie's full subtitle document on every streamed update.
type translationRecord struct {
	Task     *translationTask   `json:"task,omitempty"`
	Event    *translator.Event  `json:"event,omitempty"`
	Result   *translator.Result `json:"result,omitempty"`
	Error    string             `json:"error,omitempty"`
	Finished bool               `json:"finished,omitempty"`
}

func translationProgressPath(path string) string {
	return filepath.Join(path, "translation-progress.jsonl")
}

func writeTranslationRecord(path string, record translationRecord, reset bool) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if reset {
		return subtitle.Replace(translationProgressPath(path), data)
	}
	file, err := os.OpenFile(translationProgressPath(path), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

// A torn last record was never displayed (events are synced before display).
// Earlier records remain usable. Corruption inside the journal is reported.
func readTranslationProgress(path string) (*translationTask, *translationLive, *translator.Result, bool, error) {
	file, err := os.Open(translationProgressPath(path))
	if err != nil {
		return nil, nil, nil, false, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var task *translationTask
	var result *translator.Result
	live := newTranslationLive()
	finished := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, nil, nil, false, readErr
		}
		var record translationRecord
		if err = json.Unmarshal(line, &record); err != nil {
			return nil, nil, nil, false, err
		}
		if record.Task != nil {
			task = record.Task
			live.model = task.Config.Model
		}
		if record.Event != nil {
			live.apply(*record.Event)
		}
		if record.Result != nil {
			result = record.Result
			live.resume = result.Resume
		}
		if record.Error != "" {
			live.stopped = record.Error
		}
		finished = record.Finished
	}
	if task == nil {
		return nil, nil, nil, false, errors.New(locales.TranslationResumeStale())
	}
	if live.stopped == "" && !finished {
		live.stopped = locales.TranslationInterrupted()
	}
	return task, live, result, finished, nil
}

func (l *translationLive) checkpointEvent() translator.Event {
	event := translator.Event{Stage: l.stage, Done: l.done, Total: l.total, Units: l.units, Resume: l.resume, Requests: l.requests, Input: l.input, Output: l.output}
	for i, text := range l.texts {
		event.Updates = append(event.Updates, translator.UnitText{Index: i, Text: text, Final: l.final[i]})
	}
	sort.Slice(event.Updates, func(i, j int) bool { return event.Updates[i].Index < event.Updates[j].Index })
	return event
}

func (a *NativeApp) retryTranslation() {
	if a.busy || a.translationTask == nil || a.translationLive == nil || a.snapshot == nil {
		return
	}
	a.commitEdit(true)
	task := *a.translationTask
	if task.ProjectID != a.snapshot.Manifest.ID || task.Revision != a.snapshot.Revision {
		a.errorText = locales.TranslationResumeStale()
		return
	}
	if a.settings.ProviderURL == "" || a.settings.ProviderModel == "" {
		a.settingsOpen, a.settingsTab = true, 2
		return
	}
	// The explicit Retry click authorizes using the currently selected service.
	task.Config.Endpoint, task.Config.API, task.Config.Model = a.settings.ProviderURL, a.settings.ProviderAPI, a.settings.ProviderModel
	task.Config.MaxRequests = int(a.settings.MaxRequests)
	task.Config.MaxOutputTokens = 0
	if info := a.settings.ModelInfo(); info != nil {
		task.Config.MaxOutputTokens = info.MaxOutputTokens
	}
	task.CredentialScope = a.settings.CredentialScope()
	if p := a.settings.Provider(); p != nil {
		task.ProviderName = p.Name
	}
	a.executeTranslationTask(&task, a.translationLive)
}

func (a *NativeApp) completeTranslationRecovery() {
	if a.translationTask == nil {
		return
	}
	if err := writeTranslationRecord(a.translationTask.ProjectPath, translationRecord{Finished: true}, false); err != nil {
		a.errorText = locales.TranslationProgressWriteError(err.Error())
	}
	a.translationTask, a.translationLive = nil, nil
	a.settings.TranslationRecovery = ""
	a.saveSettings()
}

// Restore the interrupted task on startup without sending a request. Saved
// hashes retain the file-level unsaved state independently from project edits.
func (a *NativeApp) restoreTranslationProgress() {
	path := a.settings.TranslationRecovery
	if path == "" {
		return
	}
	a.run(locales.TranslationInterrupted(), func(context.Context) (func(), error) {
		task, live, result, finished, err := readTranslationProgress(path)
		if err != nil {
			return nil, errors.New(locales.TranslationRecoveryError(err.Error()))
		}
		store, err := project.Open(path)
		if err != nil {
			return nil, errors.New(locales.TranslationRecoveryError(err.Error()))
		}
		state, err := store.Snapshot()
		if err != nil {
			store.Close()
			return nil, err
		}
		if state.Manifest.ID != task.ProjectID {
			store.Close()
			return nil, errors.New(locales.TranslationResumeStale())
		}
		// The transaction may have finished just before the process stopped.
		if result != nil && state.Revision == task.Revision+1 && reflect.DeepEqual(state.Document, result.Document) {
			finished = true
		}
		url, previewErr := a.preview.Register(state.Manifest.MediaPath)
		return func() {
			a.replaceProject(store, state, url)
			a.savedHash, a.savedPath = task.SavedHash, task.SavedPath
			a.refreshUnsaved()
			a.subtitleTab = 1
			a.loadDraft()
			if !finished {
				a.translationTask, a.translationLive = task, live
				a.settings.TranslationRecovery = path
				a.status = locales.TranslationInterrupted()
				a.saveSettings()
			} else {
				a.settings.TranslationRecovery = ""
				a.saveSettings()
			}
			if previewErr != nil {
				a.errorText = previewErr.Error()
			}
		}, nil
	})
}

func (a *NativeApp) rememberTranslationTask(task *translationTask, live *translationLive) error {
	if a.settingsError {
		return errors.New(locales.ErrSettingsNeedRepair())
	}
	event := live.checkpointEvent()
	if err := writeTranslationRecord(task.ProjectPath, translationRecord{Task: task, Event: &event}, true); err != nil {
		return errors.New(locales.TranslationProgressWriteError(err.Error()))
	}
	a.settings.TranslationRecovery = task.ProjectPath
	if err := settings.Save(filepath.Join(a.dataRoot, "settings.json"), a.settings); err != nil {
		return errors.New(locales.TranslationProgressWriteError(err.Error()))
	}
	return nil
}
