package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

func localLauncherLocations(ctx context.Context) launcherLocations {
	var locations launcherLocations
	for _, location := range []struct {
		root      registry.Key
		sub, name string
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"},
		{registry.LOCAL_MACHINE, `Software\WOW6432Node\Valve\Steam`, "InstallPath"},
		{registry.LOCAL_MACHINE, `Software\Valve\Steam`, "InstallPath"},
	} {
		key, err := registry.OpenKey(location.root, location.sub, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		path, _, err := key.GetStringValue(location.name)
		_ = key.Close()
		if err == nil && filepath.IsAbs(path) {
			locations.steam = append(locations.steam, filepath.Clean(path))
		}
	}
	for _, variable := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if directory := os.Getenv(variable); directory != "" {
			locations.steam = append(locations.steam, filepath.Join(directory, "Steam"))
		}
	}
	if directory := os.Getenv("ProgramData"); directory != "" {
		locations.epic = append(locations.epic, filepath.Join(directory, "Epic", "EpicGamesLauncher", "Data", "Manifests"))
	}
	mask, _, _ := kernel.NewProc("GetLogicalDrives").Call()
	for index := 0; index < 26; index++ {
		if mask&(1<<index) == 0 {
			continue
		}
		drive := string(rune('A'+index)) + `:\`
		name, _ := syscall.UTF16PtrFromString(drive)
		kind, _, _ := kernel.NewProc("GetDriveTypeW").Call(uintptr(unsafe.Pointer(name)))
		if kind != 2 && kind != 3 {
			continue
		}
		locations.xbox = append(locations.xbox, filepath.Join(drive, "XboxGames"))
		raw, _ := readLauncherFile(filepath.Join(drive, ".GamingRoot"))
		locations.xbox = append(locations.xbox, xboxRootPaths(drive, raw)...)
	}
	locations.packages = registeredXboxPackages(ctx)
	return locations
}

func registeredXboxPackages(parent context.Context) []string {
	root := os.Getenv("SystemRoot")
	if !filepath.IsAbs(root) {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	const query = `$ErrorActionPreference = 'SilentlyContinue'; [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); $paths = @(Get-AppxPackage -PackageTypeFilter Main | ForEach-Object { if ($_.InstallLocation -and (Test-Path -LiteralPath (Join-Path $_.InstallLocation 'MicrosoftGame.config') -PathType Leaf)) { $_.InstallLocation } }); ConvertTo-Json -InputObject $paths -Compress`
	command := exec.CommandContext(ctx, filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", query)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	raw, err := command.Output()
	var paths []string
	if err != nil || json.Unmarshal(raw, &paths) != nil {
		return nil
	}
	return paths
}
