package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Language string

const (
	English Language = "en"
	Chinese Language = "zh-CN"
)

func Parse(value string) (Language, bool) {
	switch strings.ToLower(strings.ReplaceAll(value, "_", "-")) {
	case "en":
		return English, true
	case "zh", "zh-cn":
		return Chinese, true
	default:
		return English, false
	}
}

func FromEnvironment() Language {
	if lang, ok := Parse(os.Getenv("TERMIHOMO_LANG")); ok {
		return lang
	}

	return English
}

func (l Language) Text(message string) string {
	if l == Chinese {
		if translated, ok := zh[message]; ok {
			return translated
		}
	}
	return message
}

// Message carries a Termihomo diagnostic to the user-facing boundary.
type Message string

func (m Message) Error() string { return string(m) }

type formatted struct {
	message string
	args    []any
	cause   error
}

func Errorf(message string, args ...any) error {
	return formatted{message: message, args: args}
}

func Wrap(message string, cause error) error {
	return formatted{message: message, cause: cause}
}

func (e formatted) Error() string { return English.Error(e) }
func (e formatted) Unwrap() error { return e.cause }

func (l Language) Error(err error) string {
	switch e := err.(type) {
	case Message:
		return l.Text(string(e))
	case formatted:
		args := e.args
		if e.cause != nil {
			args = []any{l.Error(e.cause)}
		}
		return fmt.Sprintf(l.Text(e.message), args...)
	default:
		return err.Error()
	}
}

// Saved reads the language shared by the interface and command-line tools.
func Saved(dir string) Language {
	body, err := os.ReadFile(filepath.Join(dir, "language"))
	if err == nil {
		if language, ok := Parse(strings.TrimSpace(string(body))); ok {
			return language
		}
	}
	return English
}

func Save(dir string, language Language) error {
	file, err := os.CreateTemp(dir, ".language-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(string(language) + "\n")
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(dir, "language"))
}
