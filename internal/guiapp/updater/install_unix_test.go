//go:build !windows

package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnixInstallerRestoresDestinationWhenReplacementFails(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	stageDir := filepath.Join(root, "stage")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(stageDir, "new-program")
	destination := filepath.Join(root, "program")
	pending := filepath.Join(root, "pending.json")
	if err := os.WriteFile(source, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old-program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pending, []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}

	failMarker := filepath.Join(root, "replacement-failed")
	fakeMove := `#!/bin/sh
if [ "$2" = "$NETGAP_TEST_DESTINATION" ] && [ "$1" != "$NETGAP_TEST_BACKUP" ] && [ ! -e "$NETGAP_TEST_FAIL_MARKER" ]; then
  : > "$NETGAP_TEST_FAIL_MARKER"
  exit 1
fi
exec /bin/mv "$@"
`
	movePath := filepath.Join(binDir, "mv")
	if err := os.WriteFile(movePath, []byte(fakeMove), 0o700); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(root, "installer.sh")
	if err := os.WriteFile(scriptPath, []byte(unixInstallerScript), 0o700); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("/bin/sh", scriptPath, "2147483647", source, destination, "binary", stageDir, pending)
	command.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"NETGAP_TEST_DESTINATION="+destination,
		"NETGAP_TEST_BACKUP="+destination+".update-backup",
		"NETGAP_TEST_FAIL_MARKER="+failMarker,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("installer unexpectedly succeeded; output: %s", output)
	}
	if _, err := os.Stat(failMarker); err != nil {
		t.Fatalf("replacement failure was not injected: %v", err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("restored destination is missing: %v; installer output: %s", err, output)
	}
	if strings.TrimSpace(string(contents)) != "old-program" {
		t.Fatalf("destination contents after rollback = %q, want old program", contents)
	}
	if _, err := os.Stat(destination + ".update-backup"); !os.IsNotExist(err) {
		t.Fatalf("backup remains after rollback: %v", err)
	}
	if _, err := os.Stat(stageDir); err != nil {
		t.Fatalf("failed update should retain staged payload: %v", err)
	}
	if _, err := os.Stat(pending); err != nil {
		t.Fatalf("failed update should retain pending metadata: %v", err)
	}
}
