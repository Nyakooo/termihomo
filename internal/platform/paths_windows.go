package platform

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/windows"
)

func ensurePrivateDirectory(path string) error {
	path = filepath.Clean(path)
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	filename, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(filename)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		sd, descriptorErr := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + current.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
		if descriptorErr != nil {
			return descriptorErr
		}
		sa := windows.SecurityAttributes{SecurityDescriptor: sd}
		sa.Length = uint32(unsafe.Sizeof(sa))
		if err := windows.CreateDirectory(filename, &sa); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return err
		}
		attrs, err = windows.GetFileAttributes(filename)
	}
	if err != nil {
		return err
	}
	invalid := i18n.Message("application directories must be private directories")
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return invalid
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	trusted := func(sid *windows.SID) bool {
		return sid.Equals(current.User.Sid) || sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trusted(owner) {
		return invalid
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return invalid
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if !trusted((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
				return invalid
			}
		default:
			return invalid
		}
	}
	return nil
}
