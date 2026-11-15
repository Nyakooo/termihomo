package core

import (
	"context"
	"unsafe"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/windows"
)

const tunDevice = "termihomo"

func isManagedTUN(name string) bool { return name == tunDevice }

var errTUNCapability = i18n.Message("Mihomo needs administrator privileges to enable TUN")

func (state instance) checkTUNPermission() error {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(state.PID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var token windows.Token
	if err := windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	if !token.IsElevated() {
		return errTUNCapability
	}
	return nil
}

func (m Manager) authorizeTUN(_ context.Context, state instance, _ Overview) (instance, error) {
	return state, errTUNCapability
}

var getProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func ResidentBytes(pid int) int64 {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(handle)
	var counters struct {
		Size, PageFaultCount uint32
		PeakWorkingSet, WorkingSet, QuotaPeakPagedPool, QuotaPagedPool,
		QuotaPeakNonPagedPool, QuotaNonPagedPool, Pagefile, PeakPagefile uintptr
	}
	counters.Size = uint32(unsafe.Sizeof(counters))
	if ok, _, _ := getProcessMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size)); ok == 0 {
		return 0
	}
	return int64(counters.WorkingSet)
}
