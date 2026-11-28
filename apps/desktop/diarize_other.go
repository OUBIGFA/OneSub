//go:build !windows

package main

import "errors"

func runDiarize(args []string) error {
	return errors.New("speaker analysis is available in the Windows build")
}
