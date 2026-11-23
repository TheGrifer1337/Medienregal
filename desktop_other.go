//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

func lockInstance() (func(), error) { return func() {}, nil }
func showError(message string)      { fmt.Println(message) }
func openWindow(address, _ string) (<-chan struct{}, error) {
	return nil, exec.Command("xdg-open", address).Start()
}
func applyUpdate(string) error { return nil }
