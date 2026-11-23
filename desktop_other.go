//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

func lockInstance() (func(), error) { return func() {}, nil }
func showError(message string)      { fmt.Println(message) }
func runWindow(address, _ string, quit <-chan struct{}, _ <-chan bool) error {
	if err := openExternal(address); err != nil {
		return err
	}
	<-quit
	return nil
}
func openExternal(address string) error { return exec.Command("xdg-open", address).Start() }
func applyUpdate(string) error          { return nil }
