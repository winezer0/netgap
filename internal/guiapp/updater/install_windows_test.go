//go:build windows

package updater

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsInstallerRestoresDestinationWhenRestartFails(t *testing.T) {
	root := t.TempDir()
	stageDir := filepath.Join(root, "stage")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(stageDir, "new-program.exe")
	destination := filepath.Join(root, "program.exe")
	pending := filepath.Join(root, "pending.json")
	if err := os.WriteFile(source, []byte("new-program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old-program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pending, []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}
	configData, err := json.Marshal(windowsInstallConfig{
		ParentPID:   2147483647,
		Source:      source,
		Destination: destination,
		StageDir:    stageDir,
		PendingFile: pending,
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "installer.json")
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatal(err)
	}
	// Shadow Start-Process in this isolated invocation to inject a restart
	// failure after the new binary has replaced the old one.
	script := "$script:netgapStartCount = 0\nfunction Start-Process { [CmdletBinding()] param([string]$FilePath) $script:netgapStartCount++; if ($script:netgapStartCount -eq 1) { throw 'injected restart failure' } }\n" + windowsInstallerScript
	scriptPath := filepath.Join(root, "installer.ps1")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath, configPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run installer rollback: %v\n%s", err, output)
	}
	if ctx.Err() != nil {
		t.Fatalf("installer rollback timed out: %v", ctx.Err())
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("restored destination is missing: %v", err)
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
