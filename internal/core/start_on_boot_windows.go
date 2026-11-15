package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

func bootTask() (string, *user.User, error) {
	current, err := user.Current()
	if err != nil {
		return "", nil, err
	}
	return "Termihomo-" + current.Uid, current, nil
}

func taskCommand(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "schtasks.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.CombinedOutput()
}

func (m Manager) startOnBoot() (bool, error) {
	name, _, err := bootTask()
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := taskCommand(ctx, "/Query", "/TN", name, "/XML")
	if _, missing := err.(*exec.ExitError); missing && ctx.Err() == nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !strings.Contains(string(bytes.ReplaceAll(out, []byte{0}, nil)), "<Enabled>false</Enabled>"), nil
}

func (m Manager) setStartOnBoot(ctx context.Context, enabled bool) error {
	name, current, err := bootTask()
	if err != nil {
		return err
	}
	args := []string{"/Delete", "/F", "/TN", name}
	if enabled {
		task, err := m.writeBootTask(current.Uid)
		if err != nil {
			return err
		}
		defer os.Remove(task)
		args = []string{"/Create", "/F", "/TN", name, "/XML", task}
	} else {
		exists, err := m.startOnBoot()
		if !exists || err != nil {
			return err
		}
	}
	if out, err := taskCommand(ctx, args...); err != nil {
		return i18n.Errorf("Could not change startup before login: %s", strings.TrimSpace(string(out)+" "+err.Error()))
	}
	return nil
}

func (m Manager) writeBootTask(sid string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	escape := func(s string) string {
		var escaped bytes.Buffer
		_ = xml.EscapeText(&escaped, []byte(s))
		return escaped.String()
	}
	args := []string{"--boot-start", m.Paths.Config, m.Paths.Data, m.Paths.Logs}
	for i := range args {
		args[i] = syscall.EscapeArg(args[i])
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers><BootTrigger><Enabled>true</Enabled></BootTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>%s</UserId><LogonType>S4U</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
  <Settings><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><Enabled>true</Enabled></Settings>
  <Actions Context="Author"><Exec><Command>%s</Command><Arguments>%s</Arguments><WorkingDirectory>%s</WorkingDirectory></Exec></Actions>
</Task>`, escape(sid), escape(exe), escape(strings.Join(args, " ")), escape(filepath.Dir(exe)))
	data := []byte{0xff, 0xfe}
	for _, unit := range utf16.Encode([]rune(body)) {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}
	file, err := os.CreateTemp(m.Paths.Data, ".boot-task-*.xml")
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}
