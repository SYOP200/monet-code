"""Self-update for prebuilt Monet Code installs.

`monet update` replaces the TUI binary and the bundled Python CLI with the
assets from a GitHub release. It is deliberately dependency-free (stdlib only)
so it can run from the bundle it is about to replace, and it verifies every
download against the release's SHA-256 checksums.
"""

from __future__ import annotations

import hashlib
import os
import platform
import re
import shutil
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Dict, Optional, Tuple

from . import __version__

DEFAULT_REPO = "SYOP200/monet-code"
BINARY_NAME = "monet-tui"
MARKER = ".monet-code-install"
USER_AGENT = f"monet-code/{__version__}"


class UpdateError(RuntimeError):
    """A problem the user can fix, such as a missing install or checksum."""


@dataclass
class UpdateResult:
    current: str
    latest: str
    updated: bool = False


# --------------------------------------------------------------------------- #
# versions, markers and asset names
# --------------------------------------------------------------------------- #


def normalize_tag(tag: str) -> str:
    tag = tag.strip()
    if not tag:
        return ""
    return tag if tag.startswith("v") else "v" + tag


def version_tuple(tag: str) -> Tuple[int, ...]:
    base = tag.strip().lstrip("v").split("-", 1)[0]
    parts = re.findall(r"\d+", base)
    return tuple(int(p) for p in parts[:3]) or (0,)


def is_newer(latest: str, current: str) -> bool:
    return version_tuple(latest) > version_tuple(current)


def parse_marker(text: str) -> Dict[str, str]:
    data: Dict[str, str] = {}
    for line in text.splitlines():
        if "=" in line:
            key, value = line.split("=", 1)
            data[key.strip()] = value.strip()
    return data


def parse_checksums(text: str) -> Dict[str, str]:
    sums: Dict[str, str] = {}
    for line in text.splitlines():
        parts = line.split()
        if len(parts) >= 2:
            sums[parts[-1].lstrip("*")] = parts[0].lower()
    return sums


def asset_names(system: Optional[str] = None, machine: Optional[str] = None) -> Tuple[str, str]:
    """Return the TUI asset name and the platform label for this machine."""
    system = (system or sys.platform).lower()
    if system.startswith("darwin"):
        os_name = "darwin"
    elif system.startswith("linux"):
        os_name = "linux"
    else:
        raise UpdateError(f"self-update is not supported on {system}")
    machine = (machine or platform.machine()).lower()
    if machine in ("x86_64", "amd64"):
        arch = "amd64"
    elif machine in ("arm64", "aarch64"):
        arch = "arm64"
    else:
        raise UpdateError(f"no prebuilt binary is published for {machine}")
    return f"{BINARY_NAME}-{os_name}-{arch}", f"{os_name} {arch}"


def install_root() -> Path:
    """Find the install tree that owns the running CLI."""
    env = os.environ.get("MONET_HOME")
    if env:
        return Path(env).expanduser()

    candidates = []
    here = Path(__file__).resolve()
    if len(here.parents) >= 3:
        candidates.append(here.parents[2])
    xdg = os.environ.get("XDG_DATA_HOME")
    if xdg:
        candidates.append(Path(xdg).expanduser() / "monet-code")
    candidates.append(Path.home() / ".local" / "share" / "monet-code")

    for candidate in candidates:
        if (candidate / MARKER).is_file():
            return candidate
    return candidates[0]


def load_marker(root: Path) -> Dict[str, str]:
    marker = root / MARKER
    if not marker.is_file():
        raise UpdateError(
            "this Monet Code copy was not installed from a release. "
            "Re-run the installer to switch to updates."
        )
    try:
        data = parse_marker(marker.read_text(encoding="utf-8"))
    except OSError as exc:
        raise UpdateError(f"could not read {marker}: {exc}")
    if data.get("mode") != "release":
        raise UpdateError(
            "this is a source install; re-run the release installer to enable monet update."
        )
    if not (root / "bin" / BINARY_NAME).is_file():
        raise UpdateError(f"{root / 'bin' / BINARY_NAME} is missing; re-run the installer.")
    return data


# --------------------------------------------------------------------------- #
# network
# --------------------------------------------------------------------------- #


def _open(url: str, timeout: int = 60):
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    return urllib.request.urlopen(request, timeout=timeout)


def fetch_latest_tag(repo: str, timeout: int = 15) -> str:
    url = f"https://github.com/{repo}/releases/latest"
    try:
        with _open(url, timeout=timeout) as response:
            final = response.geturl()
    except (urllib.error.URLError, OSError) as exc:
        raise UpdateError(f"could not check for releases: {exc}")
    tag = normalize_tag(final.rstrip("/").rsplit("/", 1)[-1])
    if not tag.startswith("v") or not version_tuple(tag):
        raise UpdateError(f"no published release found for {repo}")
    return tag


def download(url: str, dest: Path, timeout: int = 120) -> None:
    try:
        with _open(url, timeout=timeout) as response, open(dest, "wb") as out:
            shutil.copyfileobj(response, out, 256 * 1024)
    except (urllib.error.URLError, OSError) as exc:
        raise UpdateError(f"could not download {url}: {exc}")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(256 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def verify_asset(path: Path, sums: Dict[str, str], name: str) -> None:
    expected = sums.get(name)
    if not expected:
        raise UpdateError(f"the release does not publish a checksum for {name}")
    actual = sha256_file(path)
    if actual != expected:
        raise UpdateError(f"checksum mismatch for {name}; the download was not trusted")


# --------------------------------------------------------------------------- #
# installation
# --------------------------------------------------------------------------- #


def _safe_extract(archive: Path, dest: Path) -> None:
    dest_root = dest.resolve()
    with tarfile.open(archive, "r:gz") as bundle:
        for member in bundle.getmembers():
            target = (dest / member.name).resolve()
            if target != dest_root and dest_root not in target.parents:
                raise UpdateError("the CLI bundle contains an unsafe path")
        if hasattr(tarfile, "data_filter"):
            bundle.extractall(dest, filter="data")
        else:
            bundle.extractall(dest)


def _replace_cli(root: Path, source: Path) -> None:
    cli = root / "cli"
    new = root / "cli.new"
    old = root / "cli.old"
    for stale in (new, old):
        if stale.exists():
            shutil.rmtree(stale)
    shutil.copytree(source, new)
    if cli.exists():
        os.replace(cli, old)
    try:
        os.replace(new, cli)
    except OSError:
        if old.exists() and not cli.exists():
            os.replace(old, cli)
        raise
    if old.exists():
        shutil.rmtree(old)


def _replace_binary(root: Path, source: Path) -> None:
    binary = root / "bin" / BINARY_NAME
    if binary.is_symlink():
        raise UpdateError(
            "the TUI is linked to a custom binary (MONET_TUI_BIN); "
            "update it yourself or re-run the installer."
        )
    staged = binary.with_name(BINARY_NAME + ".new")
    shutil.copyfile(source, staged)
    staged.chmod(0o755)
    os.replace(staged, binary)


def _write_marker(root: Path, repo: str, tag: str) -> None:
    marker = root / MARKER
    staged = marker.with_name(MARKER + ".new")
    content = (
        f"repo={repo}\n"
        "mode=release\n"
        f"tag={tag}\n"
        f"ref={tag}\n"
        f"installed={time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())}\n"
    )
    staged.write_text(content, encoding="utf-8")
    os.replace(staged, marker)


def run_update(
    root: Optional[Path] = None,
    repo: Optional[str] = None,
    tag: Optional[str] = None,
    force: bool = False,
    check: bool = False,
    base: Optional[str] = None,
    log: Optional[Callable[[str], None]] = None,
) -> UpdateResult:
    """Check for or install the latest release. Returns what happened."""
    root = Path(root) if root else install_root()
    marker = load_marker(root)
    repo = repo or marker.get("repo") or DEFAULT_REPO
    current = normalize_tag(marker.get("tag") or __version__)
    latest = normalize_tag(tag) if tag else fetch_latest_tag(repo)
    log = log or (lambda _message: None)

    if check:
        return UpdateResult(current, latest)
    if latest == current and not force:
        return UpdateResult(current, latest)

    if (root / "bin" / BINARY_NAME).is_symlink():
        raise UpdateError(
            "the TUI is linked to a custom binary (MONET_TUI_BIN); "
            "update it yourself or re-run the installer."
        )

    tui_asset, platform_label = asset_names()
    base = (base or f"https://github.com/{repo}/releases/download").rstrip("/")
    log(f"downloading {latest} for {platform_label}")

    with tempfile.TemporaryDirectory(prefix="monet-update-") as temp:
        tmp = Path(temp)
        download(f"{base}/{latest}/checksums.txt", tmp / "checksums.txt")
        sums = parse_checksums((tmp / "checksums.txt").read_text(encoding="utf-8"))

        tui_path = tmp / tui_asset
        download(f"{base}/{latest}/{tui_asset}", tui_path)
        verify_asset(tui_path, sums, tui_asset)

        cli_path = tmp / "monet-cli.tar.gz"
        download(f"{base}/{latest}/monet-cli.tar.gz", cli_path)
        verify_asset(cli_path, sums, "monet-cli.tar.gz")

        cli_tree = tmp / "cli"
        cli_tree.mkdir()
        _safe_extract(cli_path, cli_tree)
        if not (cli_tree / "monet_cli" / "__main__.py").is_file():
            raise UpdateError("the CLI bundle is incomplete")

        _replace_cli(root, cli_tree)
        _replace_binary(root, tui_path)
        _write_marker(root, repo, latest)

    return UpdateResult(current, latest, updated=True)
