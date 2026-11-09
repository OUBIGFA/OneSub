package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	lobeChoice "github.com/lobehub/lobe-mygo/pkg/choice"
	lobeDisplay "github.com/lobehub/lobe-mygo/pkg/display"
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
		// MyGo exposes HTTP status in its error text, not a typed error.
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
	if !a.commitEdit(true) {
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

// Decode the embedded product artwork once; About uses the same logo as the
// installer and tray rather than introducing a separate decorative mark.
var aboutLogo = sync.OnceValue(func() *ui.Bitmap {
	logo, err := ui.DecodeBitmap(appLogo)
	if err != nil {
		panic(err)
	}
	return logo
})

// About follows the settings reference: restrained sections, full-width rows,
// labels on the left and actions on the right. Update states share geometry.
func (a *NativeApp) aboutSettingsView(c *ui.Context) {
	ui.Column(c).WidthPercent(100).MaxWidth(em(c, 880)).AlignSelf(ui.Center).Gap(em(c, 32)).Padding(em(c, 24), 0).Children(func() {
		version := a.updates.productVersion
		if version == "" {
			version = "0.0.1"
		}
		ui.Row(c).Gap(em(c, 16)).AlignItems(ui.Center).Children(func() {
			lobeDisplay.Image(c, aboutLogo(), lobeDisplay.ImageProps{Size: em(c, 48), Contain: true, Variant: lobeDisplay.ImageBorderless, NoPreview: true, Alt: "OneSub"})
			ui.Column(c).Gap(em(c, 4)).Grow(1).Children(func() {
				ui.Text(c, "OneSub").FontSize(em(c, 24)).FontWeight(500)
				muted(c, locales.AboutVersion(version))
			})
			muted(c, locales.AboutBuild(BuildNumber)).FontSize(em(c, captionSize)).FontFeatures("tnum")
		})
		group := func(title string, content func()) {
			ui.Column(c).Gap(em(c, 12)).Children(func() {
				ui.Text(c, title).FontSize(em(c, headingSize)).FontWeight(500)
				ui.Column(c).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(c.Theme().Radius).Children(content)
			})
		}
		group(locales.AboutUpdates(), func() {
			ui.Row(c).Gap(em(c, 24)).Padding(em(c, 16)).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(em(c, 52)).Gap(em(c, 10)).Justify(ui.Center).Label(locales.AboutUpdateStatus()).Children(func() {
					muted(c, a.updateStatus()).Tooltip(a.updates.failure)
					if a.updates.phase == updateInstalling && a.updates.total > 0 && a.updates.downloaded < a.updates.total {
						ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
							fraction := float64(a.updates.downloaded) / float64(a.updates.total)
							lobeProgress(c, fraction).Grow(1).Label(locales.AboutUpdateProgress())
							ui.Text(c, locales.Percent(100*fraction)).FontFeatures("tnum").Shrink(0)
						})
					}
				})
				selected := 0
				if a.updates.release != nil && a.updates.phase != updateInstalling && a.updates.phase != updateInstalled {
					selected = 1
				}
				stableVariants(c, selected, func() {
					iconButton(c, locales.AboutCheckUpdates(), "rotate-cw").AlignSelf(ui.End).Disabled(!a.updates.enabled || a.settingsError || a.updates.phase == updateChecking || a.updates.phase == updateInstalling || a.updates.phase == updateInstalled).OnClick(a.checkUpdates)
				}, func() {
					primaryIconButton(c, locales.AboutInstallUpdate(), "download").AlignSelf(ui.End).Disabled(a.busy || a.settingsError).OnClick(func() { a.installUpdate(false) })
				}).Shrink(0)
			})
			separator(c)
			ui.Field(c, locales.AboutAutoUpdate(), func() {
				lobeChoice.Switch(c, &a.settings.AutoUpdate, lobeChoice.SwitchProps{}).Label(locales.AboutAutoUpdate()).Disabled(a.settingsError).OnChange(a.automaticUpdatesChanged)
			}).Row().Justify(ui.SpaceBetween).Gap(em(c, 24)).Padding(em(c, 16)).MinHeight(em(c, 64)).AlignItems(ui.Center)
		})
		group(locales.AboutProject(), func() {
			ui.Row(c).Gap(em(c, 16)).Padding(em(c, 16)).MinHeight(em(c, 64)).AlignItems(ui.Center).Children(func() {
				icon(c, "github").Shrink(0)
				ui.Text(c, "GitHub").Shrink(0)
				ui.Spacer(c)
				ui.Link(c, repositoryURL, repositoryURL).SingleLine().MinWidth(0)
			})
		})
	})
}
