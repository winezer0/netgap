"""Read-only network fingerprints for comparing WSL state before/after tests.

No configuration values or proxy credentials are printed. Expiring address and
route lifetimes are omitted because the kernel updates them without any tests.
"""

import hashlib
import json
import os
from pathlib import Path
import subprocess


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest()


def ip_state(*args):
    result = subprocess.run(
        ["ip", "-j", *args], check=True, capture_output=True, timeout=5
    )
    return json.loads(result.stdout)


def stable(value):
    if isinstance(value, dict):
        return {
            key: stable(item)
            for key, item in value.items()
            if key not in {"expires", "valid_life_time", "preferred_life_time", "cache"}
        }
    if isinstance(value, list):
        return sorted((stable(item) for item in value), key=lambda item: json.dumps(item, sort_keys=True))
    return value


def file_digest(path):
    file = Path(path)
    return hashlib.sha256(file.read_bytes()).hexdigest() if file.exists() else "absent"


settings = subprocess.run(
    ["gsettings", "list-recursively", "org.gnome.system.proxy"],
    capture_output=True, timeout=5,
)
print(json.dumps({
    "uid": os.geteuid(),
    "routes4": digest(stable(ip_state("-4", "route", "show", "table", "all"))),
    "routes6": digest(stable(ip_state("-6", "route", "show", "table", "all"))),
    "rules4": digest(stable(ip_state("-4", "rule", "show"))),
    "rules6": digest(stable(ip_state("-6", "rule", "show"))),
    "addresses": digest(stable(ip_state("address", "show"))),
    "dns": file_digest("/etc/resolv.conf"),
    "gnome_proxy": digest([settings.returncode, sorted(settings.stdout.decode().splitlines())]),
}, sort_keys=True))
