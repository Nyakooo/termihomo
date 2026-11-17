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
	running, err := os.Open(current)
	if err != nil {
		return renameErr
	}
	runningInfo, err := running.Stat()
	running.Close()
	if err != nil {
		return renameErr
	}

	// Windows can keep the executable mapped until this process exits.
	// Compare file identity because the running image can use an 8.3 path alias.
	file, err := os.Open(target)
	if err != nil {
		return renameErr
	}
	targetInfo, err := file.Stat()
	if err != nil || !os.SameFile(runningInfo, targetInfo) {
		file.Close()
		return renameErr
	}

	// Pass paths as data, and let the helper verify the original under the lock.
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	file.Close()
	if err != nil {
		return err
	}
	if err := startWindowsScript(replaceScript, "TERMIHOMO_UPDATE_SOURCE="+source, "TERMIHOMO_UPDATE_TARGET="+target,
		"TERMIHOMO_UPDATE_ORIGINAL="+fmt.Sprintf("%x", hash.Sum(nil)), "TERMIHOMO_UPDATE_PARENT="+strconv.Itoa(os.Getpid())); err != nil {
		return err
	}
	return ErrUpdateScheduled
}

func startWindowsScript(body string, env ...string) error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	powershell := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	script := make([]byte, 0, len(body)*2)
	for _, unit := range utf16.Encode([]rune(body)) {
		script = binary.LittleEndian.AppendUint16(script, unit)
	}
	cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(script))
	cmd.Env = append(cmd.Environ(), append(env, "GH_TOKEN=", "GITHUB_TOKEN=")...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}
