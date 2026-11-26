"""Monet Code command line interface.

`monet` on its own launches the Go/Charm TUI. The subcommands cover the parts
that are nicer in a shell: talking to Ollama, browsing the model catalog,
organizing files, and inspecting configuration.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import time
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, NoReturn

import typer
from rich import box
from rich.markdown import Markdown
from rich.panel import Panel
from rich.progress import BarColumn, DownloadColumn, Progress, SpinnerColumn, TextColumn
from rich.table import Table
from rich.text import Text
from rich.tree import Tree

from . import __version__, catalog, config, ollama, organize
from .build import ensure_binary
from .theme import BLUE, DIM, FG, GREEN, RED, TEAL, YELLOW, banner, console, swatches

SKIP_DIRS = {
    ".git", ".hg", ".svn", "node_modules", "vendor", ".venv", "venv",
    "__pycache__", ".mypy_cache", ".pytest_cache", ".ruff_cache", "dist",
    "build", ".next", ".nuxt", "target", ".idea", ".vscode", ".cache",
    "coverage", ".terraform", "Pods",
}

def skip_dir(name: str) -> bool:
    """True for build output, caches and tooling folders."""
    return name in SKIP_DIRS or name.endswith(".egg-info")


app = typer.Typer(
    add_completion=False,
    no_args_is_help=False,
    rich_markup_mode="rich",
    help=(
        f"[bold {TEAL}]◆ Monet Code[/] — a local-first AI coding agent.\n\n"
        "Run [bold]monet[/] with no arguments to open the terminal UI. "
        "Use [bold]monet ask[/] for a one-shot prompt, [bold]monet models[/] "
        "to manage Ollama models, and [bold]monet files organize[/] to tidy a folder."
    ),
)
models_app = typer.Typer(help="Manage local Ollama models and the curated catalog.", no_args_is_help=True)
files_app = typer.Typer(help="Browse, search, and organize files.", no_args_is_help=True)
config_app = typer.Typer(help="Inspect and change shared Monet Code configuration.", no_args_is_help=True)
app.add_typer(models_app, name="models")
app.add_typer(files_app, name="files")
app.add_typer(config_app, name="config")


# --------------------------------------------------------------------------- #
# helpers
# --------------------------------------------------------------------------- #


def fail(message: str) -> NoReturn:
    console.print()
    console.print(Panel(Text(message, style=FG), title="[bold]error[/]", border_style=RED, box=box.ROUNDED))
    raise typer.Exit(1)


def note(message: str, style: str = DIM) -> None:
    console.print(f"  [{style}]{message}[/]")


def ok(message: str) -> None:
    console.print(f"  [{GREEN}]✓[/] [{FG}]{message}[/]")


def human_size(num: float) -> str:
    if not num:
        return "0 B"
    value = float(num)
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if value < 1024 or unit == "TB":
            return f"{value:.0f} {unit}" if unit == "B" else f"{value:.1f} {unit}"
        value /= 1024
    return f"{value:.1f} TB"


def fmt_modified(value: str) -> str:
    if not value:
        return "—"
    try:
        dt = datetime.fromisoformat(value.replace("Z", "+00:00"))
        return dt.strftime("%Y-%m-%d")
    except ValueError:
        return value[:10]


def table(title: str, columns: List[str], **kwargs) -> Table:
    t = Table(
        title=f"[bold {TEAL}]{title}[/]",
        box=box.ROUNDED,
        border_style=TEAL,
        header_style=f"bold {BLUE}",
        title_justify="left",
        expand=kwargs.pop("expand", False),
        **kwargs,
    )
    for column in columns:
        t.add_column(column, no_wrap=column in {"model", "name", "size", "status"})
    return t


def shorten(text: str, width: int) -> str:
    text = " ".join(str(text).split())
    return text if len(text) <= width else text[: width - 1] + "…"


def load_models_or_fail() -> List[Dict]:
    try:
        return ollama.list_models()
    except ollama.OllamaError as exc:
        fail(str(exc))


# --------------------------------------------------------------------------- #
# tui + ask
# --------------------------------------------------------------------------- #


def launch_tui(directory: Path, model: Optional[str], ollama_url: Optional[str], rebuild: bool) -> None:
    try:
        binary = ensure_binary(rebuild=rebuild)
    except RuntimeError as exc:
        fail(str(exc))
    target = Path(directory).expanduser()
    try:
        target = target.resolve()
    except OSError:
        fail(f"cannot resolve {target}")
    if not target.is_dir():
        fail(f"{target} is not a directory")
    cfg = config.load()
    recent = [str(target)]
    for item in cfg.get("recent_directories", []):
        if item != str(target) and len(recent) < 8:
            recent.append(item)
    cfg["recent_directories"] = recent
    try:
        config.save(cfg)
    except OSError:
        pass
    args = [str(binary)]
    if model:
        args += ["--model", model]
    if ollama_url:
        args += ["--ollama-url", ollama_url]
    os.chdir(target)
    try:
        os.execv(str(binary), args)
    except OSError as exc:
        fail(f"could not start {binary}: {exc}")


@app.command()
def tui(
    directory: Optional[Path] = typer.Argument(None, help="Workspace directory (default: current directory)."),
    model: Optional[str] = typer.Option(None, "--model", "-m", help="Ollama model to start with."),
    ollama_url: Optional[str] = typer.Option(None, "--ollama-url", help="Ollama base URL."),
    rebuild: bool = typer.Option(False, "--rebuild", help="Force a rebuild of the Go TUI."),
) -> None:
    """Open the Monet Code terminal UI (Chat · Editor · Models · Files)."""
    launch_tui(directory or Path.cwd(), model, ollama_url, rebuild)


def _read_attachments(paths: List[Path]) -> str:
    blocks = []
    for path in paths:
        try:
            data = path.read_bytes()
        except OSError as exc:
            fail(f"cannot read {path}: {exc}")
        if b"\x00" in data[:8192]:
            note(f"skipping binary file {path}", YELLOW)
            continue
        if len(data) > 200_000:
            data = data[:200_000]
            note(f"truncated {path} at 200 KB", YELLOW)
        text = data.decode("utf-8", "replace")
        rel = path.name
        blocks.append(f'<file path="{rel}">\n{text}\n</file>')
    return "\n\n".join(blocks)


@app.command()
def ask(
    prompt: List[str] = typer.Argument(..., help="Prompt text. Quote it or pass several words."),
    model: Optional[str] = typer.Option(None, "--model", "-m", help="Model to use for this request."),
    files: Optional[List[Path]] = typer.Option(
        None, "--file", "-f", help="Attach a file as context (repeatable).", exists=True, dir_okay=False, readable=True
    ),
    system: Optional[str] = typer.Option(None, "--system", help="Override the system prompt."),
    json_out: bool = typer.Option(False, "--json", help="Emit a single JSON object instead of streaming."),
) -> None:
    """Ask a one-shot question from the shell (streams the answer)."""
    cfg = config.load()
    text = " ".join(prompt).strip()
    if not text:
        fail("empty prompt")
    try:
        chosen = ollama.resolve_model(cfg, model)
    except ollama.OllamaError as exc:
        fail(str(exc))

    sys_prompt = system or cfg.get("system_prompt") or (
        "You are Monet Code, a terminal AI coding agent. "
        f"The workspace is {Path.cwd()}. "
        "Answer in Markdown, be concise and concrete."
    )
    user = text
    attachments = _read_attachments(files or [])
    if attachments:
        user += "\n\nAttached context:\n\n" + attachments

    messages = [
        {"role": "system", "content": sys_prompt},
        {"role": "user", "content": user},
    ]

    if not json_out:
        banner()
        console.print(
            f"  [bold {TEAL}]❯[/] [bold {FG}]{shorten(text, 100)}[/]\n"
            f"  [{DIM}]{chosen} · {Path.cwd()}[/]\n"
        )
        if attachments:
            note(f"{len(files or [])} attachment(s) in context")

    content_parts: List[str] = []
    thinking_parts: List[str] = []
    started = time.time()
    try:
        for chunk in ollama.chat_stream(chosen, messages, options={"temperature": 0.3}):
            message = chunk.get("message") or {}
            thinking = message.get("thinking") or ""
            piece = message.get("content") or ""
            if thinking:
                thinking_parts.append(thinking)
            if piece:
                content_parts.append(piece)
                if not json_out:
                    sys.stdout.write(piece)
                    sys.stdout.flush()
    except KeyboardInterrupt:
        if not json_out:
            console.print()
        raise typer.Exit(130)
    except ollama.OllamaError as exc:
        fail(str(exc))

    content = "".join(content_parts)
    if json_out:
        console.print(
            json.dumps(
                {
                    "model": chosen,
                    "content": content,
                    "thinking": "".join(thinking_parts),
                    "seconds": round(time.time() - started, 2),
                },
                ensure_ascii=False,
            )
        )
        return
    console.print()
    if not content.strip():
        note("the model returned no content", YELLOW)


# --------------------------------------------------------------------------- #
# models
# --------------------------------------------------------------------------- #


@models_app.command("list")
def models_list(
    all_models: bool = typer.Option(False, "--all", "-a", help="Include catalog models that are not installed."),
) -> None:
    """List installed Ollama models (and optionally the catalog)."""
    installed = load_models_or_fail()
    try:
        version = ollama.version()
    except ollama.OllamaError:
        version = "offline"
    cfg = config.load()
    active = cfg.get("model") or ""

    banner()
    t = table("Installed models", ["", "model", "size", "params", "quant", "family", "modified"])
    for item in installed:
        details = item.get("details") or {}
        marker = Text("●", style=TEAL) if item.get("name") == active else Text("·", style=DIM)
        t.add_row(
            marker,
            Text(str(item.get("name", "?")), style=f"bold {FG}"),
            human_size(item.get("size", 0)),
            str(details.get("parameter_size") or "—"),
            str(details.get("quantization_level") or "—"),
            str(details.get("family") or "—"),
            Text(fmt_modified(str(item.get("modified_at") or "")), no_wrap=True),
        )
    if all_models:
        for entry in catalog.entries():
            if catalog.is_installed(str(entry["name"]), installed):
                continue
            t.add_row(
                Text("○", style=DIM),
                Text(str(entry["name"]), style=DIM),
                f"~{entry.get('size_gb', 0):.1f} GB",
                str(entry.get("params") or "—"),
                "—",
                str(entry.get("family") or "—"),
                Text("catalog", style=BLUE),
            )
    console.print(t)
    console.print(
        f"  [{DIM}]ollama {version} · {len(installed)} installed · active "
        f"[bold {FG}]{active or 'none'}[/] · config {config.config_path()}[/]"
    )
    if not installed:
        note("no models yet — try `monet models pull qwen2.5-coder:7b`", YELLOW)


@models_app.command("recommend")
def models_recommend() -> None:
    """Show the curated catalog of recommended coding models."""
    installed = load_models_or_fail()
    entries = catalog.entries()
    if not entries:
        fail("catalog not found (expected internal/catalog/models.json)")
    banner()
    t = table("Model catalog", ["", "model", "size", "params", "tags", "description"], expand=False)
    for entry in entries:
        present = catalog.is_installed(str(entry["name"]), installed)
        marker = Text("✓", style=GREEN) if present else Text("○", style=DIM)
        t.add_row(
            marker,
            Text(str(entry["name"]), style=f"bold {FG}" if present else FG),
            f"~{float(entry.get('size_gb') or 0):.1f} GB",
            str(entry.get("params") or "—"),
            Text(shorten(" · ".join(entry.get("tags") or []), 24), style=BLUE, no_wrap=True),
            Text(shorten(str(entry.get("description") or ""), 76), style=DIM, no_wrap=True),
        )
    console.print(t)
    note("✓ installed   ○ available    pull with: monet models pull <name>")


@models_app.command("pull")
def models_pull(
    name: str = typer.Argument(..., help="Model name, for example qwen2.5-coder:7b."),
    quiet: bool = typer.Option(False, "--quiet", "-q", help="Only print the final result."),
) -> None:
    """Download a model from Ollama with a live progress bar."""
    banner()
    if quiet:
        try:
            ollama.pull(name, None)
        except ollama.OllamaError as exc:
            fail(str(exc))
        ok(f"pulled {name}")
        return
    console.print(f"  [bold {TEAL}]pulling[/] [bold {FG}]{name}[/]\n")
    pct_holder = {"total": 0, "completed": 0}

    try:
        with Progress(
            SpinnerColumn(style=TEAL),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(bar_width=None, complete_style=TEAL, finished_style=GREEN, pulse_style=BLUE),
            DownloadColumn(),
            console=console,
            transient=False,
        ) as progress:
            task = progress.add_task(f"[{FG}]starting[/]", total=None)

            def on_progress(status: Dict) -> None:
                label = str(status.get("status") or "pulling")
                total = int(status.get("total") or 0)
                completed = int(status.get("completed") or 0)
                if total:
                    pct_holder["total"], pct_holder["completed"] = total, completed
                    progress.update(task, total=total, completed=completed, description=f"[{FG}]{label}[/]")
                else:
                    progress.update(task, description=f"[{FG}]{label}[/]")

            ollama.pull(name, on_progress)
            if pct_holder["total"]:
                progress.update(task, completed=pct_holder["total"], description=f"[{GREEN}]success[/]")
    except ollama.OllamaError as exc:
        fail(str(exc))
    ok(f"pulled {name}")
    note(f"activate it with: monet models use {name}")


@models_app.command("use")
def models_use(name: str = typer.Argument(..., help="Model to make active.")) -> None:
    """Set the default model shared by the CLI and the TUI."""
    cfg = config.load()
    config.set_model(cfg, name)
    try:
        path = config.save(cfg)
    except OSError as exc:
        fail(f"could not write config: {exc}")
    ok(f"active model → {name}")
    note(f"saved to {path}")


@models_app.command("rm")
def models_rm(
    name: str = typer.Argument(..., help="Model to delete."),
    yes: bool = typer.Option(False, "--yes", "-y", help="Do not ask for confirmation."),
) -> None:
    """Delete a model from local storage."""
    if not yes:
        typer.confirm(f"Delete {name} from disk?", abort=True)
    try:
        ollama.delete(name)
    except ollama.OllamaError as exc:
        fail(str(exc))
    cfg = config.load()
    if cfg.get("model") == name:
        cfg["model"] = ""
        try:
            config.save(cfg)
        except OSError:
            pass
        note("that model was active; active model cleared", YELLOW)
    ok(f"deleted {name}")


@models_app.command("show")
def models_show(name: str = typer.Argument(..., help="Model to inspect.")) -> None:
    """Show model metadata, parameters, and capabilities."""
    try:
        info = ollama.show(name)
    except ollama.OllamaError as exc:
        fail(str(exc))
    banner()
    details = info.get("details") or {}
    t = table(f"{name}", ["field", "value"])
    for key in ("family", "parameter_size", "quantization_level", "format", "parent_model"):
        t.add_row(key.replace("_", " "), str(details.get(key) or "—"))
    caps = info.get("capabilities") or []
    if caps:
        t.add_row("capabilities", " · ".join(str(c) for c in caps))
    model_info = info.get("model_info") or {}
    for key in ("general.architecture", "general.parameter_count", "general.context_length"):
        if key in model_info:
            t.add_row(key.split(".")[-1].replace("_", " "), str(model_info[key]))
    console.print(t)
    params = str(info.get("parameters") or "").strip()
    if params:
        console.print(Panel(Text(params, style=FG), title="[bold]parameters[/]", border_style=DIM, box=box.ROUNDED))
    entry = catalog.find(name)
    if entry:
        console.print(
            Panel(
                Text(str(entry.get("description") or ""), style=FG),
                title=f"[bold {TEAL}]{entry.get('title', name)}[/]",
                border_style=DIM,
                box=box.ROUNDED,
            )
        )


@models_app.command("ps")
def models_ps() -> None:
    """List models currently loaded in memory."""
    running = ollama.running()
    banner()
    if not running:
        note("no models are loaded right now")
        return
    t = table("Running models", ["model", "size", "vram", "expires"])
    for item in running:
        t.add_row(
            Text(str(item.get("name") or "?"), style=f"bold {FG}"),
            human_size(item.get("size", 0)),
            human_size(item.get("size_vram", 0)),
            str(item.get("expires_at") or "—")[:19].replace("T", " "),
        )
    console.print(t)


# --------------------------------------------------------------------------- #
# files
# --------------------------------------------------------------------------- #


def _tree_for(root: Path, depth: int, show_hidden: bool, max_entries: int = 400) -> Tree:
    tree = Tree(
        f"[bold {TEAL}]{root.name or str(root)}[/] [{DIM}]{root}[/]",
        guide_style=DIM,
    )
    count = {"n": 0}

    def add(node: Tree, path: Path, remaining: int) -> None:
        if remaining <= 0:
            return
        try:
            entries = sorted(
                path.iterdir(),
                key=lambda p: (not p.is_dir(), p.name.lower()),
            )
        except OSError as exc:
            node.add(Text(f"unreadable: {exc}", style=RED))
            return
        for entry in entries:
            if count["n"] >= max_entries:
                node.add(Text("… truncated", style=DIM))
                return
            if entry.name.startswith(".") and not show_hidden:
                continue
            if entry.is_dir() and skip_dir(entry.name):
                continue
            count["n"] += 1
            if entry.is_dir():
                branch = node.add(f"[bold {BLUE}]{entry.name}/[/]")
                add(branch, entry, remaining - 1)
            else:
                style = TEAL if entry.suffix in {".go", ".py", ".rs", ".ts"} else FG
                try:
                    size = human_size(entry.stat().st_size)
                except OSError:
                    size = "?"
                node.add(f"[{style}]{entry.name}[/] [{DIM}]{size}[/]")

    add(tree, root, depth)
    return tree


@files_app.command("tree")
def files_tree(
    directory: Optional[Path] = typer.Argument(None, help="Directory to print (default: current)."),
    depth: int = typer.Option(2, "--depth", "-d", min=1, max=8, help="Recursion depth."),
    hidden: bool = typer.Option(False, "--hidden", help="Include dotfiles."),
) -> None:
    """Print a directory tree with sizes."""
    root = (directory or Path.cwd()).expanduser().resolve()
    if not root.is_dir():
        fail(f"{root} is not a directory")
    banner()
    console.print(_tree_for(root, depth, hidden))


@files_app.command("stats")
def files_stats(
    directory: Optional[Path] = typer.Argument(None, help="Directory to analyze (default: current)."),
) -> None:
    """Summarize loose top-level files by category."""
    root = (directory or Path.cwd()).expanduser().resolve()
    if not root.is_dir():
        fail(f"{root} is not a directory")
    report = organize.analyze(root)
    banner()
    if report.total_files == 0:
        note("no loose top-level files to classify")
        return
    t = table("Loose files by category", ["category", "files", "size", "destination"])
    for name, files, size in report.ordered_groups:
        folder = next((f for n, f, _ in organize.CATEGORIES if n == name), "Other")
        t.add_row(name, str(len(files)), human_size(size), f"{folder}/")
    console.print(t)
    console.print(
        f"  [{DIM}]{report.total_files} files · {human_size(report.total_size)} · "
        f"{root}[/]"
    )


@files_app.command("organize")
def files_organize(
    directory: Optional[Path] = typer.Argument(None, help="Directory to organize (default: current)."),
    apply: bool = typer.Option(False, "--apply", help="Actually move files (default is a dry run)."),
    yes: bool = typer.Option(False, "--yes", "-y", help="Skip the confirmation prompt when applying."),
    show_moves: bool = typer.Option(False, "--moves", help="List every planned move."),
) -> None:
    """Sort loose top-level files into Documents/, Images/, Code/, and friends."""
    root = (directory or Path.cwd()).expanduser().resolve()
    if not root.is_dir():
        fail(f"{root} is not a directory")
    report = organize.analyze(root)
    banner()
    console.print(f"  [{DIM}]{root}[/]\n")
    if report.total_files == 0:
        ok("nothing to organize — the top level is already tidy")
        note("hidden files, symlinks and directories are never touched")
        return

    moves = organize.plan(root)
    t = table("Plan", ["category", "files", "size", "destination"])
    for name, files, size in report.ordered_groups:
        folder = next((f for n, f, _ in organize.CATEGORIES if n == name), "Other")
        t.add_row(name, str(len(files)), human_size(size), Text(f"{folder}/", style=BLUE))
    console.print(t)

    if show_moves or not apply:
        limit = len(moves) if show_moves else min(len(moves), 15)
        detail = table("Moves", ["from", "to"], expand=False)
        for move in moves[:limit]:
            detail.add_row(Text(move.rel_from, style=FG), Text(move.rel_to, style=BLUE))
        console.print(detail)
        if len(moves) > limit:
            note(f"… {len(moves) - limit} more moves (use --moves to list all)")

    if not apply:
        note(f"dry run — {len(moves)} files would move. Re-run with --apply to do it.")
        return
    if not yes:
        typer.confirm(f"Move {len(moves)} files into category folders in {root}?", abort=True)
    moved, errors = organize.apply(moves)
    if moved:
        ok(f"moved {moved} files into category folders")
    for error in errors[:8]:
        console.print(f"  [{RED}]✗[/] {error}")
    if len(errors) > 8:
        note(f"… {len(errors) - 8} more errors", RED)


@files_app.command("find")
def files_find(
    query: str = typer.Argument(..., help="Substring to look for."),
    directory: Optional[Path] = typer.Argument(None, help="Where to search (default: current)."),
    content: bool = typer.Option(False, "--content", "-c", help="Search file contents instead of names."),
    limit: int = typer.Option(40, "--limit", "-n", min=1, max=500, help="Maximum results."),
    hidden: bool = typer.Option(False, "--hidden", help="Include dotfiles and dot directories."),
) -> None:
    """Find files by name or search their contents."""
    root = (directory or Path.cwd()).expanduser().resolve()
    if not root.is_dir():
        fail(f"{root} is not a directory")
    banner()
    results: List[Path] = []
    matches: List[str] = []

    if content:
        needle = query.lower()
        for path in root.rglob("*"):
            if len(matches) >= limit:
                break
            if not path.is_file():
                continue
            if any(skip_dir(part) for part in path.relative_to(root).parts):
                continue
            if not hidden and any(part.startswith(".") and part not in {".", ".."} for part in path.relative_to(root).parts):
                continue
            try:
                if path.stat().st_size > 2_000_000:
                    continue
                data = path.read_bytes()
            except OSError:
                continue
            if b"\x00" in data[:8192]:
                continue
            for number, line in enumerate(data.decode("utf-8", "replace").splitlines(), 1):
                if needle in line.lower():
                    matches.append(f"{path.relative_to(root)}:{number}: {line.strip()[:160]}")
                    if len(matches) >= limit:
                        break
        t = table(f"Content matches for “{query}”", ["match"])
        for match in matches:
            t.add_row(Text(match, style=FG))
        console.print(t)
    else:
        needle = query.lower()
        for path in root.rglob("*"):
            if len(results) >= limit:
                break
            if not path.is_file():
                continue
            if any(skip_dir(part) for part in path.relative_to(root).parts):
                continue
            if not hidden and any(
                part.startswith(".") and part not in {".", ".."} for part in path.relative_to(root).parts
            ):
                continue
            if needle in path.name.lower():
                results.append(path)
        t = table(f"Files matching “{query}”", ["path", "size"])
        for path in sorted(results):
            try:
                size = human_size(path.stat().st_size)
            except OSError:
                size = "?"
            t.add_row(Text(str(path.relative_to(root)), style=FG), size)
        console.print(t)
    if not results and not matches:
        note("no results")


# --------------------------------------------------------------------------- #
# config
# --------------------------------------------------------------------------- #


@config_app.command("show")
def config_show() -> None:
    """Print the shared configuration file."""
    cfg = config.load()
    banner()
    t = table("Configuration", ["key", "value"])
    for key in config.KNOWN_KEYS:
        value = cfg.get(key)
        if isinstance(value, list):
            value = ", ".join(str(v) for v in value) or "—"
        if isinstance(value, bool):
            value = "on" if value else "off"
        if value in ("", None):
            value = "—"
        t.add_row(key, Text(str(value), style=FG))
    console.print(t)
    console.print(f"  [{DIM}]{config.config_path()}[/]")


@config_app.command("set")
def config_set(
    key: str = typer.Argument(..., help="Configuration key."),
    value: str = typer.Argument(..., help="New value."),
) -> None:
    """Change a configuration value (for example: model)."""
    if key not in config.KNOWN_KEYS:
        fail(f"unknown key {key!r}. Known keys: {', '.join(config.KNOWN_KEYS)}")
    cfg = config.load()
    try:
        cfg[key] = config.parse_value(key, value)
    except ValueError as exc:
        fail(f"invalid value for {key}: {exc}")
    try:
        path = config.save(cfg)
    except OSError as exc:
        fail(f"could not write config: {exc}")
    ok(f"{key} = {cfg[key]}")
    note(f"saved to {path}")


@config_app.command("path")
def config_path_cmd() -> None:
    """Print the configuration file path."""
    console.print(str(config.config_path()))


# --------------------------------------------------------------------------- #
# omnibus
# --------------------------------------------------------------------------- #


@app.command("update")
def update_cmd(
    check: bool = typer.Option(False, "--check", help="Only report whether a newer release exists."),
    force: bool = typer.Option(False, "--force", help="Reinstall even when already current."),
    tag: str = typer.Option("", "--tag", help="Install a specific release tag instead of the latest."),
) -> None:
    """Upgrade this prebuilt install to the latest release."""
    _run_update(check=check, force=force, tag=tag)


@app.command("upgrade", hidden=True)
def upgrade_cmd(
    check: bool = typer.Option(False, "--check", help="Only report whether a newer release exists."),
    force: bool = typer.Option(False, "--force", help="Reinstall even when already current."),
    tag: str = typer.Option("", "--tag", help="Install a specific release tag instead of the latest."),
) -> None:
    """Alias for update."""
    _run_update(check=check, force=force, tag=tag)


def _run_update(check: bool, force: bool, tag: str) -> None:
    from . import update as updater

    try:
        result = updater.run_update(tag=tag or None, force=force, check=check, log=note)
    except updater.UpdateError as exc:
        fail(str(exc))
        return

    if check:
        if result.latest == result.current:
            ok(f"up to date · {result.current}")
        else:
            console.print(
                f"  [{DIM}]current[/] {result.current}   [{DIM}]latest[/] {result.latest}"
            )
            note("run monet update to install it")
        return

    if result.updated:
        ok(f"updated {result.current} → {result.latest}")
        note("restart the TUI to use the new build")
    else:
        ok(f"already on the latest release · {result.current}")


@app.command("doctor")
def doctor() -> None:
    """Check the local toolchain, Ollama, TUI binary, and config."""
    banner()
    t = table("Environment", ["check", "status", "detail"], expand=False)

    def add(name: str, good: bool, detail: str, warn: bool = False) -> None:
        style = GREEN if good else (YELLOW if warn else RED)
        mark = "✓" if good else ("!" if warn else "✗")
        t.add_row(name, Text(mark, style=style), Text(shorten(detail, 66), style=DIM))

    py = f"{sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}"
    add("python", sys.version_info >= (3, 9), py)

    go = shutil.which("go")
    if go:
        go_detail = "not found"
        try:
            proc = subprocess.run([go, "version"], capture_output=True, text=True, timeout=5)
            go_detail = (proc.stdout or proc.stderr).strip()
        except (OSError, subprocess.SubprocessError) as exc:
            go_detail = str(exc)
        add("go toolchain", True, go_detail)
    else:
        t.add_row(
            "go toolchain",
            Text("–", style=DIM),
            Text("not needed for a prebuilt install", style=DIM),
        )

    try:
        binary = ensure_binary(quiet=True)
        add("tui binary", True, str(binary))
    except RuntimeError as exc:
        add("tui binary", False, str(exc))

    try:
        version = ollama.version()
        models = ollama.list_models()
        add("ollama", True, f"{version} · {len(models)} models at {config.ollama_url(config.load())}")
    except ollama.OllamaError as exc:
        add("ollama", False, str(exc))

    try:
        cfg_path = config.config_path()
        cfg = config.load()
        add("config", True, f"{cfg_path} · model {cfg.get('model') or 'none'}")
    except OSError as exc:
        add("config", False, str(exc))

    entries = catalog.entries()
    add("model catalog", bool(entries), f"{len(entries)} curated models")

    console.print(t)
    console.print(
        f"  [{DIM}]palette[/] [#000000 on #000000]  [/] [bold {FG}]#88C0D0[/]  "
        f"[bold {TEAL}]#8FBCBB[/]  [bold {BLUE}]#81A1C1[/]"
    )


@app.command("palette")
def palette_cmd() -> None:
    """Show the Monet Code colors."""
    swatches()


@app.command("version")
def version_cmd() -> None:
    """Print the version."""
    banner()
    console.print(f"  [monet]version[/] [bold {FG}]{__version__}[/]   [{DIM}]go tui + python cli[/]\n")


@app.callback(invoke_without_command=True)
def main(
    ctx: typer.Context,
    version: bool = typer.Option(
        False, "--version", "-V", is_eager=True, help="Show the version and exit."
    ),
) -> None:
    if version:
        console.print(f"[bold {TEAL}]◆ MONET CODE[/] [bold {FG}]{__version__}[/]")
        raise typer.Exit()
    if ctx.invoked_subcommand is None:
        if not sys.stdout.isatty():
            console.print(ctx.get_help())
            raise typer.Exit()
        launch_tui(Path.cwd(), None, None, False)


def entrypoint() -> None:
    app()


if __name__ == "__main__":
    entrypoint()
