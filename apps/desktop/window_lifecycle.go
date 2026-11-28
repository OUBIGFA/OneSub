package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
)

func (a *NativeApp) requestExit() {
	if a.busy {
		go func() {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Title: locales.ExitTitle(), Message: locales.ExitTaskRunning(), Buttons: []string{locales.ExitWait(), locales.ExitCancelTask(), locales.ExitBack()}, CancelButton: 2})
			if err != nil {
				return
			}
			a.win.Update(func() {
				switch result.Button {
				case 0:
					a.quitAfterTask = true
				case 1:
					a.quitAfterTask = true
					if a.cancel != nil {
						a.cancel()
					}
				}
			})
		}()
		return
	}
	a.commitProviderModelInput()
	if a.guardUnsaved(a.requestExit) {
		return
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			a.showError("project", err.Error())
			return
		}
		a.store = nil
	}
	a.saveSettings()
	if a.updates.stop != nil {
		a.updates.stop()
	}
	if a.previewCancel != nil {
		a.previewCancel()
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
	a.win.Destroy()
}
func (a *NativeApp) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayOpen(), Click: func(*mygo.MenuItem, *mygo.Window) { a.activate() }},
		mygo.Separator(),
		{Label: locales.TrayExit(), Click: func(*mygo.MenuItem, *mygo.Window) { a.requestExit() }},
	})
}
func (a *NativeApp) makeTray() error {
	image, err := trayLogo(mygo.Screen.PrimaryDisplay().ScaleFactor)
	if err != nil {
		return err
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "onesub-main", Icon: image, ToolTip: "OneSub", Menu: a.trayMenu()})
	if err != nil {
		return err
	}
	a.tray = tray
	// Windows reports every left-button release, so a double click opens the
	// window as a single click does; the right button keeps the menu.
	tray.OnClick(a.activate)
	return nil
}

// activate is shared by second launches and the tray, including minimized windows.
func (a *NativeApp) activate() {
	if a.win == nil {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
}
