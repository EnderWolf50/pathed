package main

// Writing the Machine PATH from an unelevated process: pathed starts itself again through
// UAC ("runas") with the new value on its command line, waits for it, and reads back why it
// failed if it did. The value travels in the arguments, which no other process can change
// between here and the elevated copy, rather than in a file that could be swapped.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// elevatedFlag is the hidden command the elevated copy runs.
const elevatedFlag = "--elevated-write"

// writeRequest is what the elevated copy is asked to write.
type writeRequest struct {
	Scope string
	Old   []string
	New   []string
	Type  uint32
}

func (r writeRequest) encode() (string, error) {
	b, err := json.Marshal(r)
	return base64.RawURLEncoding.EncodeToString(b), err
}

func decodeRequest(s string) (writeRequest, error) {
	var r writeRequest
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &r)
	}
	return r, err
}

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size          uint32
	mask          uint32
	hwnd          windows.Handle
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instApp       windows.Handle
	idList        uintptr
	class         *uint16
	keyClass      windows.Handle
	hotKey        uint32
	iconOrMonitor windows.Handle
	process       windows.Handle
}

var errUACDeclined = errors.New("the UAC prompt was declined")

// writeElevated writes a PATH through an elevated copy of pathed, asking UAC first.
func writeElevated(s scope, old, new pathValue) error {
	arg, err := writeRequest{s.name, old.entries, new.entries, new.typ}.encode()
	if err != nil {
		return err
	}
	if len(arg) > 30000 { // the command line holds 32767 characters
		return errors.New("this PATH is too long to hand to an elevated pathed")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// The elevated copy leaves the reason for a failure here; only a message for the user,
	// so it may live in a file.
	result := filepath.Join(os.TempDir(), fmt.Sprintf("pathed-%d-%d.txt", os.Getpid(), time.Now().UnixNano()))
	defer os.Remove(result)

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(fmt.Sprintf(`%s %s "%s"`, elevatedFlag, arg, result))
	const seeMaskNoCloseProcess, swHide = 0x40, 0
	info := shellExecuteInfo{mask: seeMaskNoCloseProcess, verb: verb, file: file, parameters: params, show: swHide}
	info.size = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	if ok, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return errUACDeclined
		}
		return fmt.Errorf("could not start an elevated pathed: %w", callErr)
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		msg, _ := os.ReadFile(result)
		if len(msg) == 0 {
			msg = []byte(fmt.Sprintf("the elevated pathed exited with %d", code))
		}
		return errors.New(strings.TrimSpace(string(msg)))
	}
	return nil
}

// runElevated is the elevated copy: write what it was asked to, report a failure.
func runElevated(arg, result string) int {
	r, err := decodeRequest(arg)
	if err == nil {
		err = errors.New("unknown scope " + r.Scope)
		for _, s := range scopes {
			if s.name == r.Scope {
				err = writeDirect(s, pathValue{r.Old, r.Type}, pathValue{r.New, r.Type})
			}
		}
	}
	if err != nil {
		os.WriteFile(result, []byte(err.Error()), 0o600)
		return 1
	}
	return 0
}
