"""Exercise the native GIO helper using a private bus and temporary keyfiles.

Usage: python3 scripts/test-gio-helper.py HELPER PACRUNNER
No root, real desktop settings, routes, DNS changes or public endpoints.
"""

import http.server
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time


def run(helper, pacrunner):
    with tempfile.TemporaryDirectory(prefix="netgap-gio-test-") as directory:
        env = os.environ.copy()
        env.update(XDG_CONFIG_HOME=directory, XDG_CURRENT_DESKTOP="GNOME",
                   GSETTINGS_BACKEND="keyfile", SUDO_UID="")
        # Tests explicitly control proxy configuration, including PAC download.
        for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"):
            env.pop(key, None)
            env.pop(key.lower(), None)

        def setting(schema, key, value):
            subprocess.run(["gsettings", "set", schema, key, value], env=env,
                           check=True, capture_output=True, timeout=5)

        def lookup(target="--snapshot", overrides=None):
            process_env = env.copy()
            if overrides:
                process_env.update(overrides)
            result = subprocess.run([helper, target], env=process_env, check=True,
                                    capture_output=True, text=True, timeout=6)
            return json.loads(result.stdout)

        schema = "org.gnome.system.proxy"
        setting(schema, "mode", "manual")
        setting(schema + ".http", "host", "127.0.0.1")
        setting(schema + ".http", "port", "18888")
        setting(schema + ".https", "host", "127.0.0.1")
        setting(schema + ".https", "port", "18889")
        setting(schema, "ignore-hosts", "['ignored.example']")
        first = lookup()
        assert not first.get("error"), first
        assert first["backend"] == "GProxyResolverGnome", first
        assert first["state"] == lookup()["state"], "unstable native fingerprint"
        assert lookup("http://via.example/")["routes"] == ["http://127.0.0.1:18888"]
        assert lookup("https://via.example/")["routes"] == ["http://127.0.0.1:18889"]
        assert lookup("http://ignored.example/")["routes"] == ["direct://"]
        unavailable = lookup(overrides={"GIO_USE_PROXY_RESOLVER": "dummy"})
        assert unavailable.get("error") and not unavailable.get("routes"), unavailable
        assert lookup("http://via.example/")["routes"] == ["http://127.0.0.1:18888"]

        class PAC(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                script = b'''function FindProxyForURL(url, host) {
                    if (host == "direct.example") return "DIRECT";
                    return "PROXY 127.0.0.1:18888; SOCKS5 127.0.0.1:18890; DIRECT";
                }'''
                self.send_response(200)
                self.send_header("Content-Type", "application/x-ns-proxy-autoconfig")
                self.send_header("Content-Length", str(len(script)))
                self.end_headers()
                self.wfile.write(script)

            def log_message(self, *_):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), PAC)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        worker = subprocess.Popen([pacrunner], env=env, stdout=subprocess.DEVNULL,
                                  stderr=subprocess.DEVNULL)
        try:
            setting(schema, "autoconfig-url", f"http://127.0.0.1:{server.server_port}/proxy.pac")
            setting(schema, "mode", "auto")
            # Wait only for our private process to own its private bus name.
            for _ in range(50):
                result = subprocess.run([
                    "gdbus", "call", "--session", "--dest", "org.freedesktop.DBus",
                    "--object-path", "/org/freedesktop/DBus", "--method",
                    "org.freedesktop.DBus.NameHasOwner", "org.gtk.GLib.PACRunner"
                ], env=env, capture_output=True, text=True, timeout=2)
                if "true" in result.stdout:
                    break
                if worker.poll() is not None:
                    raise AssertionError("private PACRunner exited")
                time.sleep(0.02)
            else:
                raise AssertionError("private PACRunner did not start")

            auto = lookup()
            assert auto["state"] != first["state"], "mode change not fingerprinted"
            for url in ("http://via.example/", "https://via.example/"):
                routes = lookup(url)
                # Preserve the candidates GIO actually returns. Some libproxy
                # versions omit a trailing DIRECT when proxy entries exist.
                assert routes.get("routes") in (
                    ["http://127.0.0.1:18888", "socks5://127.0.0.1:18890"],
                    ["http://127.0.0.1:18888", "socks5://127.0.0.1:18890", "direct://"]
                ), routes
            assert lookup("https://direct.example/")["routes"] == ["direct://"]
            assert lookup("https://ignored.example/")["routes"] == ["direct://"]
            detector_test = env.get("NETGAP_GIO_DETECTOR_TEST")
            if detector_test:
                env["NETGAP_GIO_NATIVE_TEST"] = "1"
                subprocess.run([detector_test, "-test.run=^TestGIONativeSystemRouting$", "-test.v"],
                               env=env, check=True, timeout=15)
            for _ in range(3):
                worker.terminate()
                worker.wait(timeout=3)
                failed = lookup("https://via.example/")
                assert failed.get("error") and not failed.get("routes"), failed
                worker = subprocess.Popen([pacrunner], env=env, stdout=subprocess.DEVNULL,
                                          stderr=subprocess.DEVNULL)
                for _ in range(50):
                    recovered = lookup("https://via.example/")
                    if recovered.get("routes", [None])[0] == "http://127.0.0.1:18888":
                        break
                    time.sleep(0.02)
                assert recovered.get("routes", [None])[0] == "http://127.0.0.1:18888", recovered
            print("PASS: missing native resolver rejected; PAC service failure/recovery repeated 3 times")
            if env.get("NETGAP_GIO_TEST_KDE") == "1":
                env["XDG_CURRENT_DESKTOP"] = "KDE"
                kde = Path(directory) / "kioslaverc"
                kde.write_text("[Proxy Settings]\nProxyType=1\n"
                               "httpProxy=http://127.0.0.1:18888\n"
                               "httpsProxy=http://127.0.0.1:18889\n"
                               "NoProxyFor=ignored.example\n")
                assert lookup("http://via.example/").get("routes") == ["http://127.0.0.1:18888"]
                assert lookup("https://via.example/").get("routes") == ["http://127.0.0.1:18889"]
                assert lookup("http://ignored.example/").get("routes") == ["direct://"]
                kde.write_text("[Proxy Settings]\nProxyType=2\n"
                               f"Proxy Config Script=http://127.0.0.1:{server.server_port}/proxy.pac\n")
                routes = lookup("https://via.example/")
                assert routes.get("routes", [None])[0] == "http://127.0.0.1:18888", routes
                assert lookup("https://direct.example/").get("routes") == ["direct://"]
                print("PASS: native KDE/libproxy fixed proxy, bypass and PAC")
            print("PASS: native GNOME fixed proxy, PAC HTTP/HTTPS, bypass, ordered candidates, "
                  "explicit DIRECT, stable/change fingerprints, service failure and recovery")
        finally:
            if worker.poll() is None:
                worker.terminate()
                worker.wait(timeout=3)
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)


if __name__ == "__main__":
    if len(sys.argv) == 4 and sys.argv[1] == "--private":
        run(str(Path(sys.argv[2]).resolve()), str(Path(sys.argv[3]).resolve()))
    elif len(sys.argv) == 3:
        with tempfile.TemporaryDirectory(prefix="netgap-gio-bus-") as directory:
            config = Path(directory) / "session.conf"
            # No standard_session_servicedirs: terminating our worker must not
            # activate the host's installed PACRunner on this private bus.
            config.write_text(f'''<busconfig>
                <type>session</type><listen>unix:tmpdir={directory}</listen>
                <auth>EXTERNAL</auth>
                <policy context="default">
                    <allow send_destination="*"/><allow receive_sender="*"/>
                    <allow own="*"/>
                </policy>
            </busconfig>''')
            subprocess.run(["dbus-run-session", f"--config-file={config}", "--",
                            sys.executable, __file__, "--private", *sys.argv[1:]],
                           check=True, timeout=45)
    else:
        raise SystemExit(__doc__)
