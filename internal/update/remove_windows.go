package update

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"io"
	"os"
	"strconv"
)

//go:embed remove_windows.ps1
var removeScript string

func RemoveExecutable(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}
	err = startWindowsScript(removeScript, "TERMIHOMO_REMOVE_TARGET="+path,
		"TERMIHOMO_REMOVE_HASH="+hex.EncodeToString(hash.Sum(nil)),
		"TERMIHOMO_REMOVE_PARENT="+strconv.Itoa(os.Getpid()))
	return err == nil, err
}
