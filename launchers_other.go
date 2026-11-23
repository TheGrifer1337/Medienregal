//go:build !windows

package main

import "context"

func localLauncherLocations(context.Context) launcherLocations { return launcherLocations{} }
