package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAboutUsesNativeSettingsAndRemembersAutomaticUpdates(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{900, 1400} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.settingsOpen = true
						tt := settled(a.view, int(float32(width)*scale), int(640*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						content, _ := tt.Find(locales.A11ySettingsContent())
						closeButton, _ := tt.Find(locales.ActionClose())
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							if err := tt.Click(locales.SettingsAbout()); err != nil {
								t.Fatal(err)
							}
							if a.settingsTab != 6 || !tt.HasText(repositoryURL) || !tt.HasText("GitHub") || !tt.HasText(locales.AboutVersion("0.1.0")) {
								t.Fatal("About is not a dedicated native page", tt.Texts())
							}
							insideRedesign(t, tt, locales.A11ySettingsContent(), locales.AboutCheckUpdates(), locales.AboutAutoUpdate(), repositoryURL, "GitHub", locales.AboutUpdateStatus())
							if err := tt.Click(repositoryURL); err != nil {
								t.Fatal(err)
							}
							opened := tt.OpenedURLs()
							if len(opened) == 0 || opened[len(opened)-1] != repositoryURL {
								t.Fatal("the repository link opened a different destination", opened)
							}
							if area, _ := tt.Find(locales.A11ySettingsContent()); area != content {
								t.Fatal("About resized the settings content", area, content)
							}
							if button, _ := tt.Find(locales.ActionClose()); button != closeButton {
								t.Fatal("About moved the close action")
							}
							if !a.settings.AutoUpdate {
								t.Fatal("automatic updates not enabled by default")
							}
							if err := tt.Click(locales.AboutAutoUpdate()); err != nil {
								t.Fatal(err)
							}
							prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
							if err != nil || prefs.AutoUpdate || a.settings.AutoUpdate {
								t.Fatal("explicitly disabled automatic updates did not persist", prefs.AutoUpdate, err)
							}
							if err := tt.Click(locales.AboutAutoUpdate()); err != nil {
								t.Fatal(err)
							}
							captureRedesign(t, tt, fmt.Sprintf("about-%s-%d-%g-dark%t", language, width, scale, dark))
						}
					})
				})
			}
		}
	}
}

func TestAboutUpdateStatesKeepTheFixedShell(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.settingsOpen, a.settingsTab = true, 6
			a.settings.AutoUpdate = false
			a.updates.enabled = true
			tt := settled(a.settingsView, 900, 640)
			content, _ := tt.Find(locales.A11ySettingsContent())
			closeButton, _ := tt.Find(locales.ActionClose())
			for _, phase := range []updatePhase{updateChecking, updateCurrent, updateAvailable, updateInstalling, updateInstalled, updateMissing, updateFailed, updateCancelled} {
				a.updates.phase, a.updates.downloaded, a.updates.total = phase, 32, 100
				a.updates.release = &mygo.Update{Version: "0.0.1.20261105000002"}
				tt.Frame()
				if area, _ := tt.Find(locales.A11ySettingsContent()); area != content {
					t.Fatal("update phase resized settings", phase, area, content)
				}
				if button, _ := tt.Find(locales.ActionClose()); button != closeButton {
					t.Fatal("update phase moved close action", phase)
				}
				action := locales.AboutInstallUpdate()
				if phase == updateInstalling || phase == updateInstalled {
					action = locales.AboutCheckUpdates()
				}
				insideRedesign(t, tt, locales.A11ySettingsContent(), action, locales.AboutAutoUpdate(), locales.AboutUpdateStatus(), repositoryURL)
				button, _ := tt.Find(action)
				status, _ := tt.Find(locales.AboutUpdateStatus())
				if button.Y > status.Y+status.H || button.Y+button.H < status.Y {
					t.Fatal("update action is detached from its status row", button, status)
				}
				captureRedesign(t, tt, fmt.Sprintf("about-state-%s-%d", language, phase))
				if phase == updateInstalling {
					insideRedesign(t, tt, locales.A11ySettingsContent(), locales.AboutUpdateProgress())
					if !tt.HasText(locales.Percent(32)) {
						t.Fatal("progress is not the actual byte fraction", tt.Texts())
					}
				}
			}
		})
	}
}

func TestAutomaticUpdatesRespectTaskAndPreferenceGuards(t *testing.T) {
	a := nativeForTest(t)
	a.updates.enabled, a.updates.phase, a.updates.release = true, updateAvailable, &mygo.Update{}
	if !a.automaticUpdateReady() {
		t.Fatal("an idle opted-in app did not accept the available update")
	}
	for _, flag := range []*bool{&a.busy, &a.settingsError, &a.quitAfterTask} {
		*flag = true
		if a.automaticUpdateReady() {
			t.Fatal("automatic installation ignored a workbench guard")
		}
		*flag = false
	}
	a.settings.AutoUpdate = false
	if a.automaticUpdateReady() {
		t.Fatal("automatic installation ignored opt-out")
	}
	a.settings.AutoUpdate = true
	for _, phase := range []updatePhase{updateChecking, updateCurrent, updateInstalling, updateInstalled, updateMissing, updateFailed, updateCancelled} {
		a.updates.phase = phase
		if a.automaticUpdateReady() {
			t.Fatal("automatic installation repeated or immediately retried", phase)
		}
	}
	a.updates.phase, a.updates.release = updateAvailable, nil
	if a.automaticUpdateReady() {
		t.Fatal("an absent update was accepted")
	}
}

func TestUpdateCheckReportsAbsenceFailureAndCancellationHonestly(t *testing.T) {
	a := nativeForTest(t)
	a.updates.enabled, a.busy = true, true
	for _, tc := range []struct {
		err   error
		phase updatePhase
		label string
	}{
		{nil, updateCurrent, locales.AboutUpToDate()},
		{errors.New("mygo: checking for updates: GET https://github.com/OUBIGFA/OneSub/releases/latest/download/update-windows-amd64.json: 404 Not Found"), updateMissing, locales.AboutNoUpdatePackage()},
		{errors.New("network unavailable"), updateFailed, locales.AboutCheckFailed()},
		{fmt.Errorf("checking: %w", context.Canceled), updateCancelled, locales.AboutUpdateCancelled()},
	} {
		a.completeUpdateCheck(nil, tc.err)
		if a.updates.phase != tc.phase || a.updateStatus() != tc.label || a.updates.release != nil {
			t.Fatal("incorrect check outcome", tc.err, a.updates.phase, a.updateStatus())
		}
	}
	a.completeUpdateCheck(&mygo.Update{Version: "0.0.1.20261105000002"}, nil)
	if a.updates.phase != updateAvailable || a.updateStatus() != locales.AboutUpdateWaiting() || a.automaticUpdateReady() {
		t.Fatal("a check interrupted an active task")
	}
	a.updates.enabled = false
	if a.updateStatus() != locales.AboutUpdatesUnavailable() {
		t.Fatal("a development build claimed to support updates")
	}
}

func TestOrderedUpdateVersionKeepsTheProductAndComparesBuilds(t *testing.T) {
	for build, want := range map[string]string{
		"20261105000001":     "0.0.1.20261105000001",
		"20261105000002":     "0.0.1.20261105000002",
		"native-preview":     "0.0.1",
		"native-development": "0.0.1",
		"20261105":           "0.0.1",
		"２０２６１１０５０００００１": "0.0.1",
	} {
		if got := orderedUpdateVersion("0.0.1", build); got != want {
			t.Fatal(build, got, want)
		}
	}
}

func TestAutomaticCheckScheduleStopsOnActualExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checks, stopped := make(chan struct{}, 8), make(chan struct{})
	go func() {
		scheduleUpdateChecks(ctx, time.Millisecond, 5*time.Millisecond, func() { checks <- struct{}{} })
		close(stopped)
	}()
	for range 2 {
		select {
		case <-checks:
		case <-time.After(time.Second):
			t.Fatal("automatic checking did not start or recur")
		}
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("the updater timer survived exiting")
	}
	for len(checks) > 0 {
		<-checks
	}
	select {
	case <-checks:
		t.Fatal("a check ran after the scheduler stopped")
	case <-time.After(20 * time.Millisecond):
	}
}
