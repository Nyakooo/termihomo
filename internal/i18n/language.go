package i18n

import (
	"fmt"
	"os"
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
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale := os.Getenv(name); locale != "" {
			primary := strings.SplitN(strings.ToLower(locale), ".", 2)[0]
			if primary == "zh" || strings.HasPrefix(primary, "zh_") || strings.HasPrefix(primary, "zh-") {
				return Chinese
			}
			break
		}
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
