//go:build !windows

package main

// File cleanup is currently shipped only in the Windows desktop product.
func storageOtherInstance() (bool, error) { return false, nil }
