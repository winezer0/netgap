package elevation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const parentPrefix = "--netgap-restart-parent="
const statePrefix = "--netgap-restart-state="

type handoffState struct {
	ParentPID        uint32 `json:"parentPid"`
	ResumeMonitoring bool   `json:"resumeMonitoring"`
}

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteInfo matches SHELLEXECUTEINFOW on Windows amd64 and arm64.
type shellExecuteInfo struct {
	Size                              uint32
	Mask                              uint32
	Window                            windows.Handle
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          windows.Handle
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          windows.Handle
	HotKey                            uint32
	Icon                              windows.Handle
	Process                           windows.Handle
}

func GetStatus() *Status {
	return &Status{Supported: true, IsAdmin: windows.GetCurrentProcessToken().IsElevated()}
}

func Restart(monitoringState func() bool) error {
	if GetStatus().IsAdmin {
		return errors.New("application is already running as administrator")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	stateFile, err := os.CreateTemp("", "netgap-restart-*.json")
	if err != nil {
		return fmt.Errorf("create restart handoff: %w", err)
	}
	handoffComplete := false
	defer func() {
		_ = stateFile.Close()
		if !handoffComplete {
			_ = os.Remove(stateFile.Name())
		}
	}()
	arguments := parentPrefix + strconv.Itoa(os.Getpid())
	arguments += " " + windows.EscapeArg(statePrefix+stateFile.Name())
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters, _ := windows.UTF16PtrFromString(arguments)
	directory, err := windows.UTF16PtrFromString(filepath.Dir(executable))
	if err != nil {
		return err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	comErr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	if comErr == nil || comErr == syscall.Errno(1) { // S_OK or S_FALSE
		defer windows.CoUninitialize()
	} else if comErr != syscall.Errno(0x80010106) { // RPC_E_CHANGED_MODE: COM already initialized
		return fmt.Errorf("initialize Windows shell: %w", comErr)
	}
	info := shellExecuteInfo{
		Mask: 0x40 | 0x100 | 0x400, // NOCLOSEPROCESS | NOASYNC | FLAG_NO_UI
		Verb: verb, File: file, Parameters: parameters, Directory: directory, Show: 1,
	}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, callErr := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return ErrCancelled
		}
		if callErr == syscall.Errno(0) {
			return errors.New("Windows did not launch the administrator process")
		}
		return fmt.Errorf("launch administrator process: %w", callErr)
	}
	if info.Process == 0 {
		return errors.New("Windows returned no administrator process")
	}
	defer windows.CloseHandle(info.Process)
	// Do not close the current GUI if the replacement has already failed.
	wait, err := windows.WaitForSingleObject(info.Process, 0)
	if err != nil {
		return err
	}
	if wait != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("administrator process exited before restart handoff")
	}
	// Read the live state AFTER the UAC prompt: a region match may have stopped
	// monitoring while authorization was pending. Never restart that completed run.
	state := handoffState{ParentPID: uint32(os.Getpid()), ResumeMonitoring: monitoringState()}
	if err := json.NewEncoder(stateFile).Encode(state); err != nil {
		_ = windows.TerminateProcess(info.Process, 1)
		return fmt.Errorf("write restart handoff: %w", err)
	}
	if err := stateFile.Sync(); err != nil {
		_ = windows.TerminateProcess(info.Process, 1)
		return fmt.Errorf("sync restart handoff: %w", err)
	}
	if err := stateFile.Close(); err != nil {
		_ = windows.TerminateProcess(info.Process, 1)
		return fmt.Errorf("close restart handoff: %w", err)
	}
	handoffComplete = true
	return nil
}

// Prepare waits before creating the service or taking Wails' single-instance lock.
func Prepare(args []string) (*LaunchIntent, error) {
	var parent uint64
	statePath := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, parentPrefix) {
			if parent != 0 {
				return nil, errors.New("duplicate administrator restart argument")
			}
			value, err := strconv.ParseUint(strings.TrimPrefix(arg, parentPrefix), 10, 32)
			if err != nil || value == 0 || value == uint64(os.Getpid()) {
				return nil, errors.New("invalid administrator restart parent")
			}
			parent = value
		}
		if strings.HasPrefix(arg, statePrefix) {
			if statePath != "" {
				return nil, errors.New("duplicate restart handoff argument")
			}
			statePath = strings.TrimPrefix(arg, statePrefix)
		}
	}
	if parent == 0 {
		if statePath != "" {
			return nil, errors.New("missing administrator restart parent")
		}
		return nil, nil
	}
	if !GetStatus().IsAdmin {
		return nil, errors.New("replacement process does not have administrator permission")
	}
	if !validStatePath(statePath) {
		return nil, errors.New("invalid restart handoff path")
	}
	defer func() { _ = os.Remove(statePath) }()
	if err := waitForParent(uint32(parent)); err != nil {
		return nil, err
	}
	file, err := os.Open(statePath)
	if err != nil {
		return nil, fmt.Errorf("open restart handoff: %w", err)
	}
	defer file.Close()
	var state handoffState
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("read restart handoff: %w", err)
	}
	if state.ParentPID != uint32(parent) {
		return nil, errors.New("restart handoff parent mismatch")
	}
	return &LaunchIntent{ResumeMonitoring: state.ResumeMonitoring}, nil
}

func validStatePath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	name := filepath.Base(path)
	if !strings.HasPrefix(name, "netgap-restart-") || !strings.HasSuffix(name, ".json") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, "netgap-restart-"), ".json")
	if len(id) == 0 || len(id) > 10 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func waitForParent(pid uint32) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	} // Parent has already exited.
	if err != nil {
		return fmt.Errorf("open restart parent: %w", err)
	}
	defer windows.CloseHandle(handle)
	var name [32768]uint16
	size := uint32(len(name))
	if err := windows.QueryFullProcessImageName(handle, 0, &name[0], &size); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(windows.UTF16ToString(name[:size]), executable) {
		return errors.New("restart parent is not the same executable")
	}
	wait, err := windows.WaitForSingleObject(handle, 60000)
	if err != nil {
		return err
	}
	if wait != windows.WAIT_OBJECT_0 {
		return errors.New("previous application did not exit within 60 seconds")
	}
	return nil
}

func ShowLaunchError(err error) {
	message, _ := windows.UTF16PtrFromString("Administrator restart failed: " + err.Error())
	title, _ := windows.UTF16PtrFromString("NetGap")
	_, _ = windows.MessageBox(0, message, title, windows.MB_OK|windows.MB_ICONERROR)
}
