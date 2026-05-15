//go:build !linux

package main

import "os"

// notifyUSR1 is a no-op on non-Linux platforms.
// The scheduler binary is intended for Linux deployment only.
func notifyUSR1(_ chan<- os.Signal) {}
