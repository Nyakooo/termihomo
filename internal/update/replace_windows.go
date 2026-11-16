package update

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

//go:embed replace_windows.ps1
var replaceScript string

func replaceExecutable(source, target string) error {
	renameErr := os.Rename(source, target)
	if renameErr == nil {
		return nil
	}
	current, err := os.Executable()
	if err != nil {
		return renameErr
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil || !strings.EqualFold(current, target) {
		return renameErr
	}

	// Windows can keep the executable mapped until this process exits.
	// Pass paths as data, and let the helper verify the original under the lock.
	file, err := os.Open(target)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	file.Close()
	if err != nil {
		return err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	powershell := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	script := make([]byte, 0, len(replaceScript)*2)
	for _, unit := range utf16.Encode([]rune(replaceScript)) {
		script = binary.LittleEndian.AppendUint16(script, unit)
	}
	cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(script))
	cmd.Env = append(os.Environ(), "GH_TOKEN=", "GITHUB_TOKEN=", "TERMIHOMO_UPDATE_SOURCE="+source, "TERMIHOMO_UPDATE_TARGET="+target,
		"TERMIHOMO_UPDATE_ORIGINAL="+fmt.Sprintf("%x", hash.Sum(nil)), "TERMIHOMO_UPDATE_PARENT="+strconv.Itoa(os.Getpid()))
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return ErrUpdateScheduled
}
