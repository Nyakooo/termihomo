package tui

import (
	"runtime"
	"unsafe"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/windows"
)

var (
	clipboardUser32  = windows.NewLazySystemDLL("user32.dll")
	clipboardKernel  = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard    = clipboardUser32.NewProc("OpenClipboard")
	closeClipboard   = clipboardUser32.NewProc("CloseClipboard")
	getClipboardData = clipboardUser32.NewProc("GetClipboardData")
	globalLock       = clipboardKernel.NewProc("GlobalLock")
	globalUnlock     = clipboardKernel.NewProc("GlobalUnlock")
	globalSize       = clipboardKernel.NewProc("GlobalSize")
)

var readClipboard = func() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ok, _, _ := openClipboard.Call(0); ok == 0 {
		return "", i18n.Message("System clipboard is unavailable")
	}
	defer closeClipboard.Call()
	const unicodeText = 13
	handle, _, _ := getClipboardData.Call(unicodeText)
	if handle == 0 {
		return "", nil
	}
	size, _, _ := globalSize.Call(handle)
	if size < 2 {
		return "", i18n.Message("System clipboard is unavailable")
	}
	address, _, _ := globalLock.Call(handle)
	if address == 0 {
		return "", i18n.Message("System clipboard is unavailable")
	}
	defer globalUnlock.Call(handle)
	units := make([]uint16, min(size/2, 8192))
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), address,
		(*byte)(unsafe.Pointer(&units[0])), uintptr(len(units))*2, nil); err != nil {
		return "", err
	}
	return windows.UTF16ToString(units), nil
}
