//go:build windows

package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

type windowsInstallConfig struct {
	ParentPID   int    `json:"parentPid"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	StageDir    string `json:"stageDir"`
	PendingFile string `json:"pendingFile"`
}

const windowsInstallerScript = `$ErrorActionPreference = 'Stop'
$configPath = $args[0]
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$newPath = $config.destination + '.update-tmp'
$backupPath = $config.destination + '.update-backup'
$backupCreated = $false
try {
  $deadline = (Get-Date).AddMinutes(5)
  while ((Get-Date) -lt $deadline) {
    $parent = Get-Process -Id $config.parentPid -ErrorAction SilentlyContinue
    if (-not $parent) { break }
    Start-Sleep -Milliseconds 250
  }
  if (Get-Process -Id $config.parentPid -ErrorAction SilentlyContinue) {
    throw 'Timed out waiting for the application to exit.'
  }
  Remove-Item -LiteralPath $backupPath -Force -ErrorAction SilentlyContinue
  Copy-Item -LiteralPath $config.source -Destination $newPath -Force
  if (Test-Path -LiteralPath $config.destination) {
    Move-Item -LiteralPath $config.destination -Destination $backupPath -Force
    $backupCreated = $true
  }
  Move-Item -LiteralPath $newPath -Destination $config.destination -Force
  Start-Process -FilePath $config.destination
  Remove-Item -LiteralPath $backupPath -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $config.stageDir -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $config.pendingFile -Force -ErrorAction SilentlyContinue
} catch {
  Remove-Item -LiteralPath $newPath -Force -ErrorAction SilentlyContinue
  if ($backupCreated -and (Test-Path -LiteralPath $backupPath)) {
    Remove-Item -LiteralPath $config.destination -Force -ErrorAction SilentlyContinue
    Move-Item -LiteralPath $backupPath -Destination $config.destination -Force
  }
  if ((-not (Get-Process -Id $config.parentPid -ErrorAction SilentlyContinue)) -and (Test-Path -LiteralPath $config.destination)) {
    Start-Process -FilePath $config.destination -ErrorAction SilentlyContinue
  }
} finally {
  Remove-Item -LiteralPath $configPath -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
`

func startPlatformInstaller(source, destination, kind, stageDir, pendingPath string, parentPID int) error {
	config := windowsInstallConfig{
		ParentPID:   parentPID,
		Source:      source,
		Destination: destination,
		StageDir:    stageDir,
		PendingFile: pendingPath,
	}
	configData, err := json.Marshal(config)
	if err != nil {
		return err
	}
	configFile, err := os.CreateTemp("", "netgapgui-update-*.json")
	if err != nil {
		return err
	}
	configPath := configFile.Name()
	if _, err := configFile.Write(configData); err != nil {
		_ = configFile.Close()
		_ = os.Remove(configPath)
		return err
	}
	if err := configFile.Close(); err != nil {
		_ = os.Remove(configPath)
		return err
	}
	scriptFile, err := os.CreateTemp("", "netgapgui-update-*.ps1")
	if err != nil {
		_ = os.Remove(configPath)
		return err
	}
	scriptPath := scriptFile.Name()
	if _, err := scriptFile.WriteString(windowsInstallerScript); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(configPath)
		_ = os.Remove(scriptPath)
		return err
	}
	if err := scriptFile.Close(); err != nil {
		_ = os.Remove(configPath)
		_ = os.Remove(scriptPath)
		return err
	}
	command := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath, configPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		_ = os.Remove(configPath)
		_ = os.Remove(scriptPath)
		return fmt.Errorf("start update helper: %w", err)
	}
	return command.Process.Release()
}
