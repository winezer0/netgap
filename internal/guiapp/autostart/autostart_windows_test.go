package autostart

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestRegistration(t *testing.T) {
	// Use a private test key; never touch the user's Run or StartupApproved keys.
	path := fmt.Sprintf(`Software\NetGapTests\Startup-%d-%d`, os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, path) })
	command := startupCommand(`C:\Program Files\网络工具\NetGap.exe`)
	if command != `"C:\Program Files\网络工具\NetGap.exe" --autostart` {
		t.Fatal(command)
	}
	status, err := statusAt(path, command)
	if err != nil || !status.Supported || status.Registered {
		t.Fatalf("initial: %+v, %v", status, err)
	}
	status, err = setAt(path, command, true)
	if err != nil || !status.Registered || status.NeedsRepair {
		t.Fatalf("added: %+v, %v", status, err)
	}
	status, err = statusAt(path, startupCommand(`D:\NetGap.exe`))
	if err != nil || !status.NeedsRepair {
		t.Fatalf("moved: %+v, %v", status, err)
	}
	status, err = setAt(path, startupCommand(`D:\NetGap.exe`), true)
	if err != nil || !status.Registered || status.NeedsRepair {
		t.Fatalf("repaired: %+v, %v", status, err)
	}
	for range 2 {
		status, err = setAt(path, command, false)
		if err != nil || status.Registered {
			t.Fatalf("removed: %+v, %v", status, err)
		}
	}
}
