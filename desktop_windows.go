package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var user32 = syscall.NewLazyDLL("user32.dll")

func lockInstance() (func(), error) {
	name, _ := syscall.UTF16PtrFromString("Local\\Medienregal.TheGrifer1337")
	handle, _, err := kernel.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, err
	}
	if err == syscall.ERROR_ALREADY_EXISTS {
		_ = syscall.CloseHandle(syscall.Handle(handle))
		return nil, fmt.Errorf("Medienregal ist bereits geöffnet.")
	}
	return func() { _ = syscall.CloseHandle(syscall.Handle(handle)) }, nil
}

func showError(message string) {
	msg, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString(product)
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(msg)), uintptr(unsafe.Pointer(title)), 0x30)
}

func openWindow(address, dataDir string) (<-chan struct{}, error) {
	for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if root == "" {
			continue
		}
		edge := filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe")
		if _, err := os.Stat(edge); err != nil {
			continue
		}
		cmd := exec.Command(edge, "--app="+address, "--user-data-dir="+filepath.Join(dataDir, "Fenster"),
			"--no-first-run", "--no-default-browser-check", "--disable-background-mode", "--disable-features=msEdgeSidebarV2", "--window-size=1260,850")
		if err := cmd.Start(); err != nil {
			continue
		}
		closed := make(chan struct{})
		go func() { _ = cmd.Wait(); close(closed) }()
		return closed, nil
	}
	// Windows opens the default browser when Edge is absent.
	err := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address).Start()
	return nil, err
}

func applyUpdate(installer string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	cmd := exec.Command(installer)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + installer + `" /S /UPDATE /D=` + strings.TrimRight(dir, "\\")}
	return cmd.Start()
}
