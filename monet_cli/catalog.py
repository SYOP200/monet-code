"""Load Monet Code's curated model catalog.

The canonical JSON lives at internal/catalog/models.json and is embedded into
the Go binary. When the CLI is run from a checkout we read it directly so both
front ends always describe the same models.
"""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any, Dict, List, Optional

REPO_ROOT = Path(__file__).resolve().parents[1]


def catalog_path() -> Optional[Path]:
    override = os.environ.get("MONET_CATALOG")
    candidates = []
    if override:
        candidates.append(Path(override).expanduser())
    candidates += [
        REPO_ROOT / "internal" / "catalog" / "models.json",
        Path(__file__).resolve().parent / "models.json",
    ]
    for path in candidates:
        if path.is_file():
            return path
    return None


def entries() -> List[Dict[str, Any]]:
    path = catalog_path()
    if not path:
        return []
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return []
    models = data.get("models")
    if not isinstance(models, list):
        return []
    return [m for m in models if isinstance(m, dict) and m.get("name")]


def find(name: str) -> Optional[Dict[str, Any]]:
    trimmed = name.removesuffix(":latest")
    for entry in entries():
        if str(entry.get("name", "")).removesuffix(":latest") == trimmed:
            return entry
    return None


def is_installed(catalog_name: str, installed: List[Dict[str, Any]]) -> bool:
    trimmed = catalog_name.removesuffix(":latest")
    for model in installed:
        if str(model.get("name", "")).removesuffix(":latest") == trimmed:
            return True
    return False
