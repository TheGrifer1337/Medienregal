//go:build !windows

package main

import (
	"context"
	"errors"
)

func localLauncherLocations(context.Context) launcherLocations { return launcherLocations{} }

func openInstalledGame(context.Context, entry) error {
	return errors.New("Windows launcher unavailable")
}
