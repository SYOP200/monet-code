"""Locate or build the Go TUI binary for `monet tui`."""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path
from typing import Optional

REPO_ROOT = Path(__file__).resolve().parents[1]
BINARY_NAME = "monet-tui"


def repo_binary() -> Path:
    return REPO_ROOT / "bin" / BINARY_NAME


def find_go() -> Optional[str]:
    return shutil.which("go")


def _sources() -> list:
    return list((REPO_ROOT / "cmd").rglob("*.go")) + list((REPO_ROOT / "internal").rglob("*.go"))


def _is_stale(binary: Path) -> bool:
    if not binary.exists():
        return True
    built = binary.stat().st_mtime
    return any(path.stat().st_mtime > built for path in _sources() if path.exists())


def ensure_binary(rebuild: bool = False, quiet: bool = False) -> Path:
    """Return a runnable TUI binary, building it from source when needed."""
    env_binary = os.environ.get("MONET_TUI_BIN")
    if env_binary and Path(env_binary).expanduser().is_file():
        return Path(env_binary).expanduser()
    if not (REPO_ROOT / "go.mod").is_file():
        on_path = shutil.which(BINARY_NAME)
        if on_path:
            return Path(on_path)
        raise RuntimeError(
            "monet-tui was not found. Install Monet Code from a source checkout or set MONET_TUI_BIN."
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
