"""Synthetic archive tests; no executable is launched or extracted."""

import importlib.util
import io
from pathlib import Path
import plistlib
import struct
import tarfile
import tempfile
import unittest
import zipfile
from xml.parsers.expat import ExpatError

spec = importlib.util.spec_from_file_location("native_artifacts", Path(__file__).with_name("verify-native-artifacts.py"))
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)


def executable(platform):
    data = bytearray(70)
    if platform.startswith("ubuntu"):
        data[:6] = b"\x7fELF\x02\x01"
        struct.pack_into("<H", data, 18, 62)
    elif platform.startswith("macos"):
        data[:4] = b"\xcf\xfa\xed\xfe"
        struct.pack_into("<I", data, 4, 0x0100000C if platform.endswith("arm64") else 0x01000007)
    else:
        data[:2] = b"MZ"
        struct.pack_into("<I", data, 60, 64)
        data[64:70] = b"PE\x00\x00\x64\x86"
    return bytes(data)


def archive(path, platform, gui, mode=0o755, plist=None):
    binary = executable(platform)
    if path.name.endswith(".tar.gz"):
        names = ["netgapgui" if gui else "netgap"]
        with tarfile.open(path, "w:gz") as output:
            for name in names:
                member = tarfile.TarInfo(name)
                member.size, member.mode = len(binary), mode
                output.addfile(member, io.BytesIO(binary))
    else:
        with zipfile.ZipFile(path, "w") as output:
            if platform.startswith("macos"):
                name = "netgapgui.app/Contents/MacOS/netgapgui"
                output.writestr("netgapgui.app/Contents/Info.plist", plist if plist is not None else plistlib.dumps({"CFBundleExecutable": "netgapgui"}))
            else:
                name = "netgapgui.exe" if gui else "netgap.exe"
            member = zipfile.ZipInfo(name)
            member.create_system = 3
            member.external_attr = (0o100000 | mode) << 16
            output.writestr(member, binary)


class NativeArchives(unittest.TestCase):
    def test_wails_doctype_and_binary_plists(self):
        metadata = {"CFBundleExecutable": "netgapgui"}
        xml = plistlib.dumps(metadata)
        # Match the Wails template: DOCTYPE first, without XML declaration.
        wails = xml.split(b"\n", 1)[1]
        self.assertTrue(wails.startswith(b"<!DOCTYPE"))
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "mac.zip"
            for data in (wails, plistlib.dumps(metadata, fmt=plistlib.FMT_BINARY)):
                archive(path, "macos-arm64", True, plist=data)
                self.assertEqual(len(verifier.inspect(path, "macos-arm64", True)["executables"]), 1)
            for data in (b"<!DOCTYPE plist><plist><dict>", plistlib.dumps([]), plistlib.dumps({"CFBundleExecutable": "wrong"})):
                archive(path, "macos-arm64", True, plist=data)
                with self.assertRaises((ValueError, plistlib.InvalidFileException, ExpatError)):
                    verifier.inspect(path, "macos-arm64", True)

    def test_all_platforms(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for platform in verifier.PLATFORMS:
                with self.subTest(platform=platform):
                    for gui in (False, True):
                        extension = ".zip" if platform.startswith("windows") or (gui and platform.startswith("macos")) else ".tar.gz"
                        name = ("netgapgui-" if gui else "netgap-") + platform + extension
                        archive(root / name, platform, gui)
                    result = verifier.verify(root, platform)
                    self.assertEqual(result["result"], "passed")
                    self.assertEqual(len(result["archives"]), 2)

    def test_execute_permission(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "gui.tar.gz"
            for options in ({"mode": 0o644},):
                with self.subTest(options=options):
                    archive(path, "ubuntu-24.04-amd64", True, **options)
                    with self.assertRaises(ValueError):
                        verifier.inspect(path, "ubuntu-24.04-amd64", True)
            path = Path(directory) / "mac.zip"
            archive(path, "macos-arm64", True, mode=0o644)
            with self.assertRaises(ValueError):
                verifier.inspect(path, "macos-arm64", True)

    def test_wrong_architecture_and_short_header(self):
        for platform in verifier.PLATFORMS:
            with self.subTest(platform=platform):
                with self.assertRaises(ValueError):
                    verifier.architecture(io.BytesIO(bytes(70)), platform)
                with self.assertRaises(ValueError):
                    verifier.architecture(io.BytesIO(b"MZ"), platform)
        with self.assertRaises(ValueError):
            verifier.architecture(io.BytesIO(executable("macos-intel")), "macos-arm64")

    def test_unsafe_and_duplicate_members(self):
        for names in (["../x"], ["/x"], ["C:/x"], ["x\\y"], ["x", "x"]):
            with self.subTest(names=names), self.assertRaises(ValueError):
                verifier.safe_members(names)


if __name__ == "__main__":
    unittest.main()
