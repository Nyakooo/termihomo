package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/windows"
)

const (
	internetPerConnectionOption = 75
	proxyOptionFlags            = 1
	proxyOptionServer           = 2
	proxyOptionBypass           = 3
	proxyOptionAutoURL          = 4
	proxyOptionFlagsUI          = 10
	proxyDirect                 = 1
	proxyManual                 = 2
	proxyAutoURL                = 4
)

var (
	wininet             = windows.NewLazySystemDLL("wininet.dll")
	internetSetOption   = wininet.NewProc("InternetSetOptionW")
	internetQueryOption = wininet.NewProc("InternetQueryOptionW")
	freeInternetString  = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")
)

type internetOption struct {
	kind  uint32
	value uintptr
}

type internetOptions struct {
	size          uint32
	connection    *uint16
	count, failed uint32
	options       *internetOption
}

type windowsProxy struct {
	flags                     uintptr
	server, override, autoURL string
}

func connectionOptions(options []internetOption) internetOptions {
	return internetOptions{size: uint32(unsafe.Sizeof(internetOptions{})), count: uint32(len(options)), options: &options[0]}
}

func readWindowsProxy() (windowsProxy, error) {
	options := []internetOption{{kind: proxyOptionFlagsUI}, {kind: proxyOptionServer}, {kind: proxyOptionBypass}, {kind: proxyOptionAutoURL}}
	list := connectionOptions(options)
	size := list.size
	ok, _, err := internetQueryOption.Call(0, internetPerConnectionOption, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&size)))
	for _, option := range options[1:] {
		if option.value != 0 {
			defer freeInternetString.Call(option.value)
		}
	}
	if ok == 0 {
		return windowsProxy{}, err
	}
	text := func(option *internetOption) string {
		return windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&option.value)))
	}
	return windowsProxy{flags: options[0].value, server: text(&options[1]), override: text(&options[2]), autoURL: text(&options[3])}, nil
}

func (w windowsProxy) values() []string {
	return []string{strconv.FormatUint(uint64(w.flags), 10), w.server, w.autoURL}
}

func (w windowsProxy) enabled() bool {
	return w.flags&(proxyManual|proxyAutoURL) != 0
}

func (w windowsProxy) exclusions() []string {
	return strings.FieldsFunc(w.override, func(r rune) bool { return r == ';' })
}

func (m Manager) SystemProxy(context.Context) SystemProxy {
	state, err := readWindowsProxy()
	if err != nil {
		return SystemProxy{}
	}
	owner, _ := m.proxyOwnership()
	return SystemProxy{Available: true, Enabled: state.enabled(), Owned: slices.Equal(state.values(), owner.Applied),
		Revision: proxyRevision(append(state.values(), state.override)), Exclusions: state.exclusions()}
}

func notifyWindowsProxy() {
	_, _, _ = internetSetOption.Call(0, 39, 0, 0)
	_, _, _ = internetSetOption.Call(0, 37, 0, 0)
}

func (m Manager) SetProxyExclusions(ctx context.Context, before, after []string) error {
	return m.withLock(func() error {
		state, err := readWindowsProxy()
		if err != nil {
			return err
		}
		if !slices.Equal(state.exclusions(), before) {
			return i18n.Message("System proxy exclusions changed. Review the list and try again.")
		}
		if slices.ContainsFunc(after, func(s string) bool { return strings.Contains(s, ";") }) {
			return i18n.Message("Enter one domain, IP address, or network range")
		}
		return setWindowsOptions(nil, []optionString{{proxyOptionBypass, strings.Join(after, ";")}})
	})
}

type optionString struct {
	kind uint32
	text string
}

func setWindowsOptions(options []internetOption, strings []optionString) error {
	var keepalive []*uint16
	for _, setting := range strings {
		value, err := windows.UTF16PtrFromString(setting.text)
		if err != nil {
			return err
		}
		keepalive = append(keepalive, value)
		options = append(options, internetOption{kind: setting.kind, value: uintptr(unsafe.Pointer(value))})
	}
	list := connectionOptions(options)
	ok, _, err := internetSetOption.Call(0, internetPerConnectionOption, uintptr(unsafe.Pointer(&list)), uintptr(list.size))
	runtime.KeepAlive(keepalive)
	if ok == 0 {
		return err
	}
	notifyWindowsProxy()
	return nil
}

func writeWindowsProxy(values []string) error {
	flags, err := strconv.ParseUint(values[0], 10, 32)
	if err != nil {
		return err
	}
	return setWindowsOptions([]internetOption{{proxyOptionFlags, uintptr(flags)}},
		[]optionString{{proxyOptionServer, values[1]}, {proxyOptionAutoURL, values[2]}})
}

func (m Manager) enableSystemProxy(_ context.Context, address string, replace bool) error {
	state, err := readWindowsProxy()
	if err != nil {
		return err
	}
	before := state.values()
	owner, path := m.proxyOwnership()
	owned := slices.Equal(before, owner.Applied)
	if !owned && !replace && (state.enabled()) {
		return nil
	}
	after := windowsProxy{flags: proxyDirect | proxyManual, server: address}.values()
	original := before
	if owned && len(owner.Before) == len(before) {
		original = owner.Before
	}
	if err := writeWindowsProxy(after); err != nil {
		return errors.Join(err, writeWindowsProxy(before))
	}
	body, err := json.Marshal(proxyOwnership{Before: original, Applied: after})
	if err == nil {
		err = atomicWrite(path, body)
	}
	if err != nil {
		return errors.Join(err, writeWindowsProxy(before))
	}
	return nil
}

func (m Manager) disableSystemProxy(_ context.Context, revision string) error {
	owner, path := m.proxyOwnership()
	if len(owner.Applied) == 0 && revision == "" {
		return nil
	}
	state, err := readWindowsProxy()
	if err != nil {
		return err
	}
	before := state.values()
	if revision != "" && revision != proxyRevision(append(before, state.override)) {
		return i18n.Message("System proxy changed; check its current settings and try again")
	}
	var after []string
	if slices.Equal(before, owner.Applied) && len(owner.Before) == len(before) {
		after = owner.Before
	} else if revision != "" {
		after = windowsProxy{flags: proxyDirect, server: state.server}.values()
	}
	if after != nil {
		if err := writeWindowsProxy(after); err != nil {
			return errors.Join(err, writeWindowsProxy(before))
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
