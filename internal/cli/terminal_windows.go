package cli

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/windows"
)

func isMSYSTerminal(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	handle := windows.Handle(file.Fd())
	if kind, err := windows.GetFileType(handle); err != nil || kind != windows.FILE_TYPE_PIPE {
		return false
	}
	var name struct {
		bytes uint32
		text  [512]uint16
	}
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileNameInfo,
		(*byte)(unsafe.Pointer(&name)), uint32(unsafe.Sizeof(name))); err != nil {
		return false
	}
	if n := int(name.bytes / 2); n > 0 && n <= len(name.text) {
		pipe := windows.UTF16ToString(name.text[:n])
		return (strings.HasPrefix(pipe, `\msys-`) || strings.HasPrefix(pipe, `\cygwin-`)) && strings.Contains(pipe, "-pty")
	}
	return false
}

func bridgeTerminal(args []string, stdin io.Reader, stdout, stderr io.Writer, lang i18n.Language) (bool, int) {
	if isTerminal(stdin) || !isMSYSTerminal(stdin) || !isMSYSTerminal(stdout) {
		return false, 0
	}
	winpty, err := exec.LookPath("winpty.exe")
	if err != nil {
		return true, terminalBridgeError(stderr, lang)
	}
	exe, err := os.Executable()
	if err != nil {
		return true, terminalBridgeError(stderr, lang)
	}
	cmd := exec.Command(winpty, append([]string{exe}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return true, exit.ExitCode()
		}
		return true, terminalBridgeError(stderr, lang)
	}
	return true, 0
}

func terminalBridgeError(stderr io.Writer, lang i18n.Language) int {
	_, _ = io.WriteString(stderr, lang.Text("Open Git Bash in Windows Terminal or run winpty termihomo.exe")+"\n")
	return 1
}
