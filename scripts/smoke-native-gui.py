#!/usr/bin/env python3
"""Extract a native GUI archive and verify its executable starts."""

from __future__ import annotations

import argparse
import os
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile
import time
import zipfile
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("archive", type=Path)
    parser.add_argument("platform", choices=("windows", "macos", "linux"))
    args = parser.parse_args()
    if not args.archive.is_file():
        parser.error(f"archive does not exist: {args.archive}")

    with tempfile.TemporaryDirectory(prefix="netgap-gui-smoke-") as temp:
        root = Path(temp)
        if zipfile.is_zipfile(args.archive):
            with zipfile.ZipFile(args.archive) as archive:
                archive.extractall(root)
        elif tarfile.is_tarfile(args.archive):
            with tarfile.open(args.archive) as archive:
                archive.extractall(root)
        else:
            parser.error("unsupported native archive format")

        candidates = {
            "windows": [root / "netgapgui.exe"],
            "macos": [root / "netgapgui.app/Contents/MacOS/netgapgui"],
            "linux": [root / "netgapgui"],
        }[args.platform]
        executable = next((path for path in candidates if path.is_file()), None)
        if executable is None:
            raise RuntimeError(f"GUI executable not found in archive: {candidates}")
        if args.platform != "windows":
            executable.chmod(executable.stat().st_mode | 0o111)

        command = [str(executable)]
        macos_logs = None
        if args.platform == "macos":
            macos_logs = (root / "gui-stdout.log", root / "gui-stderr.log")
            command = [
                "open",
                "-n",
                "-a",
                str(executable.parent.parent.parent),
                "--stdout",
                str(macos_logs[0]),
                "--stderr",
                str(macos_logs[1]),
            ]
        if args.platform == "linux":
            xvfb = shutil.which("xvfb-run")
            if not xvfb:
                raise RuntimeError("xvfb-run is required for Linux GUI startup smoke")
            command = [xvfb, "-a", *command]
            dbus = shutil.which("dbus-run-session")
            if not dbus:
                raise RuntimeError("dbus-run-session is required for Linux GUI startup smoke")
            command = [dbus, "--", *command]
        env = os.environ.copy()
        if args.platform == "linux":
            env.setdefault("GDK_BACKEND", "x11")

        with tempfile.TemporaryFile() as output:
            process = subprocess.Popen(
                command,
                cwd=root,
                env=env,
                stdout=output,
                stderr=subprocess.STDOUT,
                start_new_session=(args.platform != "windows"),
            )
            try:
                if args.platform == "macos":
                    expected_executable = str(executable.resolve())
                    deadline = time.monotonic() + 30
                    running = []
                    candidates = []
                    while time.monotonic() < deadline:
                        listing = subprocess.run(
                            ["ps", "-axo", "pid=,comm=,args="], capture_output=True, text=True
                        )
                        candidates = [
                            line.strip()
                            for line in listing.stdout.splitlines()
                            if "netgapgui" in line or str(root) in line
                        ]
                        for line in listing.stdout.splitlines():
                            fields = line.split(None, 2)
                            if len(fields) < 2:
                                continue
                            command_line = fields[2] if len(fields) > 2 else ""
                            if (
                                expected_executable in command_line
                                or Path(fields[1]).name == executable.name
                            ):
                                running.append(line.strip())
                        if running:
                            break
                        time.sleep(1)
                    if not running:
                        output.seek(0)
                        details = [output.read().decode("utf-8", errors="replace")]
                        details.append(f"candidate processes={candidates}")
                        for log_path in macos_logs:
                            if log_path.is_file():
                                details.append(
                                    f"{log_path.name}:\n{log_path.read_text(encoding='utf-8', errors='replace')}"
                                )
                        raise RuntimeError(
                            f"Launch Services did not keep the GUI running "
                            f"(open exit={process.poll()}, matching processes={running}):\n"
                            + "\n".join(details)
                        )
                    time.sleep(2)
                    listing = subprocess.run(["ps", "-axo", "pid=,comm=,args="], capture_output=True, text=True)
                    still_running = []
                    for line in listing.stdout.splitlines():
                        fields = line.split(None, 2)
                        if len(fields) < 2:
                            continue
                        command_line = fields[2] if len(fields) > 2 else ""
                        if expected_executable in command_line or Path(fields[1]).name == executable.name:
                            still_running.append(line.strip())
                    if not still_running:
                        output.seek(0)
                        details = [output.read().decode("utf-8", errors="replace")]
                        for log_path in macos_logs:
                            if log_path.is_file():
                                details.append(
                                    f"{log_path.name}:\n{log_path.read_text(encoding='utf-8', errors='replace')}"
                                )
                        raise RuntimeError(
                            f"GUI process exited shortly after launch "
                            f"(open exit={process.poll()}):\n" + "\n".join(details)
                        )
                elif process.poll() is not None:
                    output.seek(0)
                    detail = output.read().decode("utf-8", errors="replace")
                    raise RuntimeError(f"GUI exited during startup with {process.returncode}:\n{detail}")
                print(f"PASS: extracted {args.platform} GUI archive and observed process startup")
            finally:
                if args.platform == "windows":
                    subprocess.run(["taskkill", "/T", "/F", "/PID", str(process.pid)], check=False, capture_output=True)
                elif args.platform == "macos":
                    subprocess.run(["pkill", "-TERM", "-x", "netgapgui"], check=False, capture_output=True)
                else:
                    try:
                        os.killpg(process.pid, signal.SIGTERM)
                    except ProcessLookupError:
                        pass
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    if args.platform == "windows":
                        subprocess.run(["taskkill", "/T", "/F", "/PID", str(process.pid)], check=False, capture_output=True)
                    else:
                        try:
                            os.killpg(process.pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                    process.wait()
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as error:
        print(f"FAIL: {error}", file=sys.stderr)
        raise SystemExit(1)
