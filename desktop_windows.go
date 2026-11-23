package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
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
		return nil, fmt.Errorf("%s ist bereits geöffnet.", product)
	}
	return func() { _ = syscall.CloseHandle(syscall.Handle(handle)) }, nil
}

func showError(message string) {
	msg, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString(product)
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(msg)), uintptr(unsafe.Pointer(title)), 0x30)
}

func runWindow(address, dataDir string, quit <-chan struct{}, presentation <-chan bool) error {
	// The only embedded media frame is opened by clicking a chart title.
	const argsKey = "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"
	if args := os.Getenv(argsKey); !strings.Contains(args, "--autoplay-policy") {
		_ = os.Setenv(argsKey, strings.TrimSpace(args+" --autoplay-policy=no-user-gesture-required"))
	}
	view := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus:     true,
		DataPath:      filepath.Join(dataDir, "WebView2"),
		WindowOptions: webview2.WindowOptions{Title: product, Width: 1260, Height: 850, Center: true, IconId: 1},
	})
	if view == nil {
		return fmt.Errorf("Das Programmfenster konnte nicht geöffnet werden. Installiere oder repariere Microsoft WebView2.")
	}
	view.SetSize(680, 520, webview2.HintMin)
	view.Navigate(address)
	closed := make(chan struct{})
	fullscreen := fullscreenWindow{handle: uintptr(view.Window())}
	go func() {
		for {
			select {
			case <-quit:
				view.Destroy()
				return
			case <-closed:
				return
			case enabled := <-presentation:
				view.Dispatch(func() { fullscreen.set(enabled) })
			}
		}
	}()
	view.Run()
	close(closed)
	return nil
}

func openExternal(address string) error {
	shell := syscall.NewLazyDLL("shell32.dll")
	action, _ := syscall.UTF16PtrFromString("open")
	link, err := syscall.UTF16PtrFromString(address)
	if err != nil {
		return err
	}
	result, _, _ := shell.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(action)), uintptr(unsafe.Pointer(link)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("Windows konnte den Link nicht öffnen (%d)", result)
	}
	return nil
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
