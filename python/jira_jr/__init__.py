"""jira-jr: Agent-friendly Jira CLI binary installer."""

import os
import platform
import subprocess
import sys
import tarfile
import urllib.request
import zipfile
from io import BytesIO
from pathlib import Path

REPO = "sofq/jira-cli"

PLATFORM_MAP = {
    "Darwin": "darwin",
    "Linux": "linux",
    "Windows": "windows",
}

ARCH_MAP = {
    "x86_64": "amd64",
    "AMD64": "amd64",
    "aarch64": "arm64",
    "arm64": "arm64",
}


def _get_version():
    from importlib.metadata import version
    return version("jira-jr")


def _get_binary_dir():
    return Path(__file__).parent / "bin"


def _get_binary_path():
    binary = "jr.exe" if platform.system() == "Windows" else "jr"
    return _get_binary_dir() / binary


def _download_url(version, plat, arch):
    ext = "zip" if plat == "windows" else "tar.gz"
    name = f"jira-cli_{version}_{plat}_{arch}.{ext}"
    return f"https://github.com/{REPO}/releases/download/v{version}/{name}"


def _install_binary():
    binary_path = _get_binary_path()
    if binary_path.exists():
        return binary_path

    version = _get_version()
    system = PLATFORM_MAP.get(platform.system())
    arch = ARCH_MAP.get(platform.machine())

    if not system or not arch:
        print(
            f"Unsupported platform: {platform.system()}/{platform.machine()}",
            file=sys.stderr,
        )
        sys.exit(1)

    url = _download_url(version, system, arch)
    binary_name = "jr.exe" if system == "windows" else "jr"

    print(f"Downloading jr v{version} for {system}/{arch}...", file=sys.stderr)

    response = urllib.request.urlopen(url)
    data = response.read()

    bin_dir = _get_binary_dir()
    bin_dir.mkdir(parents=True, exist_ok=True)

    if system == "windows":
        with zipfile.ZipFile(BytesIO(data)) as zf:
            for name in zf.namelist():
                if name == binary_name or name.endswith(f"/{binary_name}"):
                    with zf.open(name) as src, open(binary_path, "wb") as dst:
                        dst.write(src.read())
                    break
    else:
        with tarfile.open(fileobj=BytesIO(data), mode="r:gz") as tf:
            for member in tf.getmembers():
                if member.name == binary_name or member.name.endswith(f"/{binary_name}"):
                    f = tf.extractfile(member)
                    if f:
                        binary_path.write_bytes(f.read())
                    break

    binary_path.chmod(0o755)
    print(f"Installed jr to {binary_path}", file=sys.stderr)
    return binary_path


def main():
    binary = _install_binary()
    result = subprocess.run([str(binary)] + sys.argv[1:])
    sys.exit(result.returncode)
