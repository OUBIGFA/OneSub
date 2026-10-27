package main

import "github.com/egoist/mygo"

// File dialogs own their own Cancel action. Do not expose a worker cancellation
// button while no worker is running; keep the workbench geometry unchanged.
func (a *NativeApp) openDialog(options mygo.OpenDialogOptions) ([]string, error) {
	a.win.Update(func() { a.choosingFile = true })
	defer a.win.Update(func() { a.choosingFile = false })
	return mygo.Dialog.Open(options)
}
func (a *NativeApp) saveDialog(options mygo.SaveDialogOptions) (string, error) {
	a.win.Update(func() { a.choosingFile = true })
	defer a.win.Update(func() { a.choosingFile = false })
	return mygo.Dialog.Save(options)
}
