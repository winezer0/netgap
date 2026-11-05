//go:build !windows

package updater

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const unixInstallerScript = `#!/bin/sh
set -eu
PARENT_PID="$1"
SOURCE="$2"
DESTINATION="$3"
KIND="$4"
STAGE_DIR="$5"
PENDING_FILE="$6"
TEMP_PATH="${DESTINATION}.update-tmp-$$"
BACKUP_PATH="${DESTINATION}.update-backup"
BACKUP_CREATED=0

restore_on_failure() {
  STATUS=$?
  trap - EXIT
  if [ "$STATUS" -ne 0 ]; then
    rm -rf "$TEMP_PATH"
    if [ "$BACKUP_CREATED" -eq 1 ]; then
      rm -rf "$DESTINATION"
      mv "$BACKUP_PATH" "$DESTINATION" 2>/dev/null || true
    fi
    if ! kill -0 "$PARENT_PID" 2>/dev/null; then
      if [ "$KIND" = "app" ]; then
        /usr/bin/open -n "$DESTINATION" >/dev/null 2>&1 || true
      elif [ -x "$DESTINATION" ]; then
        "$DESTINATION" >/dev/null 2>&1 &
      fi
    fi
  fi
  rm -f "$0"
  exit "$STATUS"
}
trap restore_on_failure EXIT

ATTEMPT=0
while kill -0 "$PARENT_PID" 2>/dev/null; do
  ATTEMPT=$((ATTEMPT + 1))
  if [ "$ATTEMPT" -ge 1200 ]; then
    echo "Timed out waiting for the application to exit." >&2
    exit 1
  fi
  sleep 0.25
done

rm -rf "$BACKUP_PATH"
if [ "$KIND" = "app" ]; then
  /usr/bin/ditto "$SOURCE" "$TEMP_PATH"
else
  cp -p "$SOURCE" "$TEMP_PATH"
fi
if [ -e "$DESTINATION" ]; then
  mv "$DESTINATION" "$BACKUP_PATH"
  BACKUP_CREATED=1
fi
mv "$TEMP_PATH" "$DESTINATION"
if [ "$KIND" = "app" ]; then
  /usr/bin/open -n "$DESTINATION"
else
  "$DESTINATION" >/dev/null 2>&1 &
fi
BACKUP_CREATED=0
rm -rf "$BACKUP_PATH" "$STAGE_DIR"
rm -f "$PENDING_FILE"
`

func startPlatformInstaller(source, destination, kind, stageDir, pendingPath string, parentPID int) error {
	script, err := os.CreateTemp("", "netgapgui-update-*.sh")
	if err != nil {
		return err
	}
	scriptPath := script.Name()
	if _, err := script.WriteString(unixInstallerScript); err != nil {
		_ = script.Close()
		_ = os.Remove(scriptPath)
		return err
	}
	if err := script.Close(); err != nil {
		_ = os.Remove(scriptPath)
		return err
	}
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		_ = os.Remove(scriptPath)
		return err
	}
	command := exec.Command("/bin/sh", scriptPath, strconv.Itoa(parentPID), source, destination, kind, stageDir, pendingPath)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("start update helper: %w", err)
	}
	return command.Process.Release()
}
