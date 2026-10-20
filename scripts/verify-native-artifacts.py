"""Validate native archives without extracting files or executing binaries."""

import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import plistlib
import struct
import tarfile
import zipfile

PLATFORMS = ("windows-amd64", "ubuntu-22.04-amd64", "ubuntu-24.04-amd64", "macos-intel", "macos-arm64")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def architecture(stream, platform):
    header = stream.read(64)
    require(len(header) == 64, "short executable header")
    if platform.startswith("ubuntu"):
        require(header[:6] == b"\x7fELF\x02\x01", "expected 64-bit little-endian ELF")
        require(struct.unpack_from("<H", header, 18)[0] == 62, "expected ELF amd64")
        return "linux/amd64"
    if platform.startswith("macos"):
        require(header[:4] == b"\xcf\xfa\xed\xfe", "expected native 64-bit Mach-O")
        cpu = 0x0100000C if platform.endswith("arm64") else 0x01000007
        require(struct.unpack_from("<I", header, 4)[0] == cpu, "wrong Mach-O CPU")
        return "darwin/arm64" if platform.endswith("arm64") else "darwin/amd64"
    require(header[:2] == b"MZ", "expected Windows executable")
    offset = struct.unpack_from("<I", header, 60)[0]
    require(64 <= offset <= 1 << 20, "invalid PE header offset")
    stream.seek(offset)
    pe = stream.read(6)
    require(len(pe) == 6 and pe[:4] == b"PE\x00\x00", "invalid PE signature")
    require(struct.unpack_from("<H", pe, 4)[0] == 0x8664, "expected PE amd64")
    return "windows/amd64"


def safe_members(names):
    require(len(names) == len(set(names)), "duplicate archive members")
    for name in names:
        path = PurePosixPath(name)
        require(not path.is_absolute() and ".." not in path.parts and "\\" not in name
                and "\x00" not in name and ":" not in name, "unsafe archive path")


def inspect(path, platform, gui):
    if platform.startswith("ubuntu"):
        names = ["netgapgui" if gui else "netgap", "netgap-gio-proxy"]
    elif platform.startswith("macos"):
        names = ["netgapgui.app/Contents/MacOS/netgapgui"] if gui else ["netgap"]
    else:
        names = ["netgapgui.exe" if gui else "netgap.exe"]
    result = {"file": path.name, "bytes": path.stat().st_size, "executables": []}
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b""):
            digest.update(chunk)
    result["sha256"] = digest.hexdigest()

    if path.name.endswith(".tar.gz"):
        with tarfile.open(path, "r:gz") as archive:
            safe_members(archive.getnames())
            require(set(archive.getnames()) == set(names), "unexpected tar members")
            for name in names:
                member = archive.getmember(name)
                require(member.isfile() and member.mode & 0o100, "missing regular executable or owner execute permission")
                with archive.extractfile(member) as stream:
                    arch = architecture(stream, platform)
                result["executables"].append({"name": name, "mode": oct(member.mode), "architecture": arch})
    else:
        with zipfile.ZipFile(path) as archive:
            safe_members(archive.namelist())
            require(archive.testzip() is None, "corrupt ZIP member")
            if platform.startswith("macos"):
                info = plistlib.loads(archive.read("netgapgui.app/Contents/Info.plist"))
                require(info.get("CFBundleExecutable") == "netgapgui", "wrong app executable")
            else:
                require(set(archive.namelist()) == set(names), "unexpected ZIP members")
            for name in names:
                member = archive.getinfo(name)
                mode = member.external_attr >> 16
                if platform.startswith("macos"):
                    require(mode & 0o100 and mode & 0o170000 == 0o100000, "missing regular macOS executable or owner execute permission")
                with archive.open(member) as stream:
                    arch = architecture(stream, platform)
                result["executables"].append({"name": name, "mode": oct(mode), "architecture": arch})
    return result


def verify(directory, platform):
    require(platform in PLATFORMS, "unsupported platform")
    archives = []
    for gui in (False, True):
        extension = ".zip" if platform.startswith("windows") or (gui and platform.startswith("macos")) else ".tar.gz"
        name = ("netgapgui-" if gui else "netgap-") + platform + extension
        archives.append(inspect(directory / name, platform, gui))
    return {"platform": platform, "result": "passed", "scope": "archive structure, CRC, executable permissions, native architecture and SHA256; no binary execution", "archives": archives}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--platform", choices=PLATFORMS, required=True)
    args = parser.parse_args()
    print(json.dumps(verify(args.directory, args.platform), indent=2))
