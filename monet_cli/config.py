"""Shared configuration for the Python CLI and the Go TUI.

Both runtimes read ~/.config/monet/config.json (or $MONET_CONFIG_DIR/config.json)
so model selection made in one is visible in the other.
"""

from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path
from typing import Any, Dict

DEFAULTS: Dict[str, Any] = {
    "model": "",
    "recent_models": [],
    "recent_directories": [],
    "editor_tab_size": 4,
    "ollama_url": "",
    "system_prompt": "",
    "show_thinking": True,
    "organize_folders": True,
}

KNOWN_KEYS = tuple(DEFAULTS.keys())


def config_dir() -> Path:
    override = os.environ.get("MONET_CONFIG_DIR")
    if override:
        return Path(override).expanduser()
    return Path.home() / ".config" / "monet"


def config_path() -> Path:
    return config_dir() / "config.json"


def load() -> Dict[str, Any]:
    path = config_path()
    cfg = dict(DEFAULTS)
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return cfg
    except (OSError, json.JSONDecodeError):
        # Keep the broken file for the user; the TUI does the same.
        try:
            path.with_suffix(".json.bak").write_text(path.read_text(encoding="utf-8"), encoding="utf-8")
        except OSError:
            pass
        return cfg
    if isinstance(data, dict):
        cfg.update(data)
    if not isinstance(cfg.get("editor_tab_size"), int) or cfg["editor_tab_size"] <= 0:
        cfg["editor_tab_size"] = 4
    return cfg


def save(cfg: Dict[str, Any]) -> Path:
    directory = config_dir()
    directory.mkdir(parents=True, exist_ok=True)
    path = config_path()
    fd, tmp = tempfile.mkstemp(dir=str(directory), prefix=".config-", suffix=".json")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            json.dump(cfg, fh, indent=2)
            fh.write("\n")
        os.replace(tmp, path)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)
    return path


def set_model(cfg: Dict[str, Any], name: str) -> None:
    name = name.strip()
    if not name:
        return
    cfg["model"] = name
    recent = [name]
    for item in cfg.get("recent_models", []):
        if item != name and len(recent) < 8:
            recent.append(item)
    cfg["recent_models"] = recent


def ollama_url(cfg: Dict[str, Any]) -> str:
    raw = (cfg.get("ollama_url") or os.environ.get("OLLAMA_HOST") or "http://127.0.0.1:11434").strip()
    if "://" not in raw:
        raw = "http://" + raw
    return raw.rstrip("/")


def parse_value(key: str, raw: str) -> Any:
    """Coerce a CLI value into the type used by the config default."""
    default = DEFAULTS.get(key)
    if isinstance(default, bool):
        return raw.strip().lower() in {"1", "true", "yes", "on"}
    if isinstance(default, int):
        return int(raw)
    if isinstance(default, list):
        return [part.strip() for part in raw.split(",") if part.strip()]
    return raw
