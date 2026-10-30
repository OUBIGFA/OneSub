package main

// A success only dismisses an error from the same operation. Independent
// background successes (settings autosave, player polling) must not erase an
// unrelated failure. The status bar's existing timeout remains a fallback.
func (a *NativeApp) showError(scope, message string) {
	if message == "" {
		a.clearError(scope)
		return
	}
	a.errorScope, a.errorText = scope, message
}

func (a *NativeApp) clearError(scope string) {
	if a.errorScope == scope {
		a.errorText, a.errorShown, a.errorScope = "", "", ""
	}
}
