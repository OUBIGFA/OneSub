package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const repositoryURL = "https://github.com/OUBIGFA/OneSub"

type updatePhase uint8

const (
	updateIdle updatePhase = iota
	updateChecking
	updateCurrent
	updateAvailable
	updateInstalling
	updateInstalled
	updateMissing
	updateFailed
	updateCancelled
)

// MyGo owns downloading, signature verification and replacement. This state
// only coordinates the native settings page with the workbench task owner.
type appUpdates struct {
	phase          updatePhase
	productVersion string
	enabled        bool
	release        *mygo.Update
	failure        string
	downloaded     int64
	total          int64
	automatic      bool
	ctx            context.Context
	stop           context.CancelFunc
}

// Product and installer versions stay stable during 0.0.1 testing. MyGo's
// numeric version comparison uses a fourth component to order signed builds.
// A +build suffix is ignored and would strand same-product-version updates.
func orderedUpdateVersion(product, build string) string {
	if len(build) == 14 {
		if _, err := strconv.ParseUint(build, 10, 64); err == nil {
			return product + "." + build
		}
	}
	return product
}

func (a *NativeApp) startUpdates() {
	a.updates.productVersion = mygo.App.Version()
	mygo.App.SetVersion(orderedUpdateVersion(a.updates.productVersion, BuildNumber))
	a.updates.enabled = mygo.Updater.Enabled()
	a.updates.ctx, a.updates.stop = context.WithCancel(context.Background())
	mygo.App.OnQuit(a.updates.stop)
	if !a.updates.enabled {
		return
	}
	// Only the UI thread reads preferences. Closing to tray keeps this timer;
	// an actual exit cancels it, without a persistent updater or restart job.
	go scheduleUpdateChecks(a.updates.ctx, 10*time.Second, 24*time.Hour, func() {
		a.win.Update(func() {
			if a.settings.AutoUpdate && !a.settingsError {
				a.checkUpdates()
			}
		})
	})
}

func scheduleUpdateChecks(ctx context.Context, delay, interval time.Duration, check func()) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			check()
			timer.Reset(interval)
		}
	}
}

func (a *NativeApp) checkUpdates() {
	if !a.updates.enabled || a.settingsError {
		return
	}
	if a.updates.phase == updateChecking || a.updates.phase == updateInstalling || a.updates.phase == updateInstalled {
		return
	}
	a.updates.phase, a.updates.failure = updateChecking, ""
	a.updates.release = nil
	ctx, cancel := context.WithTimeout(a.updates.ctx, 45*time.Second)
	go func() {
		defer cancel()
		release, err := mygo.Updater.Check(ctx)
		a.win.Update(func() { a.completeUpdateCheck(release, err) })
	}()
}

func (a *NativeApp) completeUpdateCheck(release *mygo.Update, err error) {
	a.updates.release, a.updates.failure = release, ""
	switch {
	case errors.Is(err, context.Canceled):
		a.updates.phase = updateCancelled
	case err != nil:
		a.updates.phase, a.updates.failure = updateFailed, err.Error()
		// MyGo 0.7.4 exposes HTTP status in its error text, not a typed error.
		// A missing release/manifest is not evidence of being up to date.
		if strings.HasSuffix(err.Error(), ": 404 Not Found") {
			a.updates.phase = updateMissing
		}
	case release == nil:
		a.updates.phase = updateCurrent
	default:
		a.updates.phase = updateAvailable
		a.installAutomaticUpdate()
	}
}

func (a *NativeApp) automaticUpdateReady() bool {
	return a.updates.enabled && a.updates.phase == updateAvailable && a.updates.release != nil &&
		a.settings.AutoUpdate && !a.settingsError && !a.busy && !a.quitAfterTask
}

func (a *NativeApp) installAutomaticUpdate() {
	if a.automaticUpdateReady() {
		a.installUpdate(true)
	}
}

func (a *NativeApp) installUpdate(automatic bool) {
	if a.busy || a.updates.release == nil || a.updates.phase == updateInstalling || a.updates.phase == updateInstalled || a.settingsError {
		return
	}
	release := a.updates.release
	a.updates.phase, a.updates.automatic = updateInstalling, automatic
	a.updates.downloaded, a.updates.total, a.updates.failure = 0, 0, ""
	// Use the existing cancellable task owner: no worker starts against
	// resources while MyGo replaces them, and pending work finishes first.
	a.run(locales.AboutInstallUpdate(), func(ctx context.Context) (func(), error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
		defer cancel()
		err := release.Install(ctx, func(done, total int64) {
			a.win.Update(func() { a.updates.downloaded, a.updates.total = done, total })
		})
		if err != nil {
			a.win.Update(func() {
				a.updates.phase, a.updates.failure = updateFailed, err.Error()
				if errors.Is(err, context.Canceled) {
					a.updates.phase = updateCancelled
				}
			})
			if errors.Is(err, context.Canceled) {
				return nil, err
			}
			return nil, fmt.Errorf("%s: %w", locales.AboutInstallFailed(), err)
		}
		return func() {
			a.updates.phase = updateInstalled
			a.status = locales.AboutUpdateInstalled()
			// Do not relaunch: the current project, unsaved subtitles and
			// normal exit guards remain under the user's control.
		}, nil
	})
}

func (a *NativeApp) automaticUpdatesChanged() {
	a.saveSettings()
	if !a.settings.AutoUpdate {
		if a.updates.phase == updateInstalling && a.updates.automatic && a.cancel != nil {
			a.cancel()
		}
		return
	}
	if a.updates.phase == updateAvailable {
		a.installAutomaticUpdate()
	} else {
		a.checkUpdates()
	}
}

func (a *NativeApp) updateStatus() string {
	if !a.updates.enabled {
		return locales.AboutUpdatesUnavailable()
	}
	switch a.updates.phase {
	case updateChecking:
		return locales.AboutChecking()
	case updateCurrent:
		return locales.AboutUpToDate()
	case updateAvailable:
		if a.settings.AutoUpdate && a.busy {
			return locales.AboutUpdateWaiting()
		}
		return locales.AboutUpdateAvailable()
	case updateInstalling:
		if a.updates.total > 0 && a.updates.downloaded >= a.updates.total {
			return locales.AboutVerifyingUpdate()
		}
		return locales.AboutDownloadingUpdate()
	case updateInstalled:
		return locales.AboutUpdateInstalled()
	case updateMissing:
		return locales.AboutNoUpdatePackage()
	case updateFailed:
		if a.updates.release != nil {
			return locales.AboutInstallFailed()
		}
		return locales.AboutCheckFailed()
	case updateCancelled:
		return locales.AboutUpdateCancelled()
	}
	return ""
}

// About follows the existing flat native fieldsets. The status/progress area
// is reserved, so checking, finding and installing updates never resize it.
func (a *NativeApp) aboutSettingsView(c *ui.Context) {
	settingsGroup(c, "OneSub", func() {
		version := a.updates.productVersion
		if version == "" {
			version = "0.0.1"
		}
		ui.Row(c).Gap(em(c, 20)).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.AboutVersion(version))
			muted(c, locales.AboutBuild(BuildNumber)).Tooltip(BuildNumber)
		})
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			icon(c, "github").Shrink(0).Label("GitHub")
			ui.Link(c, repositoryURL, repositoryURL).SingleLine().MinWidth(0).Tooltip(repositoryURL)
		})
	})
	settingsGroup(c, locales.AboutUpdates(), func() {
		ui.Row(c).Gap(em(c, 12)).Wrap().Children(func() {
			iconButton(c, locales.AboutCheckUpdates(), "rotate-cw").Disabled(!a.updates.enabled || a.settingsError || a.updates.phase == updateChecking || a.updates.phase == updateInstalling || a.updates.phase == updateInstalled).OnClick(a.checkUpdates)
			if a.updates.release != nil && a.updates.phase != updateInstalling && a.updates.phase != updateInstalled {
				iconButton(c, locales.AboutInstallUpdate(), "download").Disabled(a.busy || a.settingsError).OnClick(func() { a.installUpdate(false) })
			}
		})
		ui.Checkbox(c, &a.settings.AutoUpdate, locales.AboutAutoUpdate()).Disabled(a.settingsError).OnChange(a.automaticUpdatesChanged)
		ui.Column(c).Height(em(c, 72)).Gap(em(c, 8)).Label(locales.AboutUpdateStatus()).Children(func() {
			muted(c, a.updateStatus()).Tooltip(a.updates.failure)
			if a.updates.phase == updateInstalling && a.updates.total > 0 && a.updates.downloaded < a.updates.total {
				ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
					fraction := float64(a.updates.downloaded) / float64(a.updates.total)
					ui.Progress(c, fraction).Grow(1).Label(locales.AboutUpdateProgress())
					ui.Text(c, locales.Percent(100*fraction)).FontFeatures("tnum").Shrink(0)
				})
			}
		})
	})
}
