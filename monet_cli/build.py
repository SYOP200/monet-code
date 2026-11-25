"""Locate or build the Go TUI binary for `monet tui`."""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Optional

BINARY_NAME = "monet-tui"


def _repo_root() -> Path:
    """Return the checkout that owns the TUI sources.

    A normal source install keeps the Python package next to ``cmd/`` and
    ``internal/``. The curl installer instead puts a pip-installed copy in a
    virtualenv, so it exports ``MONET_REPO_DIR`` (or leaves the binary in
    ``<install>/bin``) and we resolve the TUI from there.
    """
    env_root = os.environ.get("MONET_REPO_DIR")
    if env_root:
        candidate = Path(env_root).expanduser()
        if (candidate / "go.mod").is_file() or (candidate / "bin" / BINARY_NAME).is_file():
            return candidate
    return Path(__file__).resolve().parents[1]


REPO_ROOT = _repo_root()


def repo_binary() -> Path:
    return REPO_ROOT / "bin" / BINARY_NAME


def find_go() -> Optional[str]:
    return shutil.which("go")


def _sources() -> list:
    return list((REPO_ROOT / "cmd").rglob("*.go")) + list(
        (REPO_ROOT / "internal").rglob("*.go")
    )


def _is_stale(binary: Path) -> bool:
    if not binary.exists():
        return True
    built = binary.stat().st_mtime
    return any(path.stat().st_mtime > built for path in _sources() if path.exists())


def _installed_binary() -> Optional[Path]:
    """Find the TUI next to a virtualenv created by install.sh."""
    candidates = [Path(sys.prefix).parent / "bin" / BINARY_NAME]
    for parent in Path(sys.executable).absolute().parents:
        candidates.append(parent / "bin" / BINARY_NAME)
    for candidate in candidates:
        if candidate.is_file():
            return candidate
    return None


def ensure_binary(rebuild: bool = False, quiet: bool = False) -> Path:
    """Return a runnable TUI binary, building it from source when needed."""
    env_binary = os.environ.get("MONET_TUI_BIN")
    if env_binary and Path(env_binary).expanduser().is_file():
        return Path(env_binary).expanduser()

    if not (REPO_ROOT / "go.mod").is_file():
        installed = _installed_binary()
        if installed:
            return installed
        on_path = shutil.which(BINARY_NAME)
        if on_path:
            return Path(on_path)
        raise RuntimeError(
            "monet-tui was not found. Re-run the Monet Code installer or set MONET_TUI_BIN."
        )

    binary = repo_binary()
    if not rebuild and not _is_stale(binary):
        return binary
    go = find_go()
    if not go:
        raise RuntimeError(
            "Go is required to build the Monet Code TUI. Install Go from https://go.dev/dl/ and retry."
        )
    binary.parent.mkdir(parents=True, exist_ok=True)
    if not quiet:
        print(f"  building {BINARY_NAME} → {binary.relative_to(REPO_ROOT)}", flush=True)
    proc = subprocess.run(
        [go, "build", "-o", str(binary), "./cmd/monet-tui"],
        cwd=str(REPO_ROOT),
        text=True,
        capture_output=True,
    )
    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout).strip()
        raise RuntimeError("failed to build the TUI:\n" + detail[-2000:])
    return binary
