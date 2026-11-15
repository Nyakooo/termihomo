package platform

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/windows"
)

func (p Paths) lock(name string) (*os.File, error) {
	path := filepath.Join(p.Data, name)
	filename, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(filename, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(handle, &info)
	if err == nil && info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		err = i18n.Message("instance lock must be a private regular file")
	}
	if err == nil {
		var offset windows.Overlapped
		err = windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &offset)
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			err = ErrInstanceBusy
		}
	}
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
