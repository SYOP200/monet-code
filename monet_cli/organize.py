"""File organizer shared in spirit with the Go TUI.

Only top-level regular files are considered. Hidden files, directories and
symlinks are left untouched. Planning creates no directories; apply does.
"""

from __future__ import annotations

import os
import stat
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, Iterable, List, Tuple

CATEGORIES: List[Tuple[str, str, Tuple[str, ...]]] = [
    ("Documents", "Documents", ("pdf", "doc", "docx", "txt", "md", "markdown", "rtf", "pages", "epub", "odt", "tex", "rst")),
    ("Images", "Images", ("png", "jpg", "jpeg", "gif", "svg", "webp", "heic", "tiff", "bmp", "ico", "avif")),
    ("Video", "Video", ("mp4", "mov", "mkv", "avi", "webm", "m4v", "flv")),
    ("Audio", "Audio", ("mp3", "wav", "flac", "aac", "ogg", "m4a", "opus")),
    ("Archives", "Archives", ("zip", "tar", "gz", "bz2", "xz", "7z", "rar", "tgz", "dmg", "iso")),
    ("Code", "Code", ("go", "py", "js", "ts", "tsx", "jsx", "rs", "c", "h", "cpp", "hpp", "cc", "java", "kt", "rb", "php", "swift", "sh", "bash", "zsh", "fish", "html", "css", "scss", "less", "json", "yaml", "yml", "toml", "xml", "sql", "lua", "pl", "r", "vue", "svelte")),
    ("Data", "Data", ("csv", "tsv", "xlsx", "xls", "jsonl", "parquet", "db", "sqlite", "sqlite3", "ndjson", "avro")),
]
OTHER = ("Other", "Other", ())


def classify(filename: str) -> Tuple[str, str]:
    ext = Path(filename).suffix.lower().lstrip(".")
    for name, folder, exts in CATEGORIES:
        if ext in exts:
            return name, folder
    return OTHER[0], OTHER[1]


@dataclass
class Move:
    src: Path
    dst: Path
    rel_from: str
    rel_to: str
    category: str
    size: int = 0


@dataclass
class Report:
    root: Path
    groups: Dict[str, List[Path]] = field(default_factory=dict)
    sizes: Dict[str, int] = field(default_factory=dict)
    total_files: int = 0
    total_size: int = 0

    @property
    def ordered_groups(self) -> List[Tuple[str, List[Path], int]]:
        order = {name: i for i, (name, _, _) in enumerate(CATEGORIES)}
        order["Other"] = len(CATEGORIES)
        items = [
            (name, files, self.sizes.get(name, 0))
            for name, files in self.groups.items()
        ]
        items.sort(key=lambda item: (order.get(item[0], len(order)), item[0]))
        return items


def analyze(root: Path) -> Report:
    root = root.resolve()
    report = Report(root=root)
    with os.scandir(root) as entries:
        for entry in entries:
            if entry.name.startswith("."):
                continue
            try:
                st = entry.stat(follow_symlinks=False)
            except OSError:
                continue
            if not stat.S_ISREG(st.st_mode):
                continue
            name, _ = classify(entry.name)
            report.groups.setdefault(name, []).append(Path(entry.path))
            report.sizes[name] = report.sizes.get(name, 0) + st.st_size
            report.total_files += 1
            report.total_size += st.st_size
    for files in report.groups.values():
        files.sort(key=lambda p: p.name.lower())
    return report


def _unique(root: Path, folder: str, name: str, used: set) -> Tuple[Path, str]:
    directory = root / folder
    suffix = Path(name).suffix
    stem = name[: -len(suffix)] if suffix else name
    candidate = directory / name
    counter = 1
    while True:
        if str(candidate) not in used and not candidate.exists():
            used.add(str(candidate))
            return candidate, str(candidate.relative_to(root))
        candidate = directory / f"{stem} ({counter}){suffix}"
        counter += 1


def plan(root: Path) -> List[Move]:
    report = analyze(root)
    moves: List[Move] = []
    used: set = set()
    for category, files, _ in report.ordered_groups:
        folder = next((f for n, f, _ in CATEGORIES if n == category), "Other")
        for src in files:
            dst, rel_to = _unique(report.root, folder, src.name, used)
            try:
                size = src.stat().st_size
            except OSError:
                size = 0
            moves.append(
                Move(
                    src=src,
                    dst=dst,
                    rel_from=src.name,
                    rel_to=rel_to,
                    category=category,
                    size=size,
                )
            )
    return moves


def apply(moves: Iterable[Move]) -> Tuple[int, List[str]]:
    moved = 0
    errors: List[str] = []
    for move in moves:
        try:
            move.dst.parent.mkdir(parents=True, exist_ok=True)
            os.replace(move.src, move.dst)
            moved += 1
        except OSError as exc:
            errors.append(f"{move.rel_from}: {exc}")
    return moved, errors
