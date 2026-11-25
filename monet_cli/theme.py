"""Shared Monet Code palette and Rich helpers for the Python CLI."""

from __future__ import annotations

from rich.console import Console
from rich.theme import Theme

# The same palette as internal/theme/theme.go.
BG = "#0D1117"
BG_PANEL = "#0F141B"
BG_RAISED = "#16202B"
FG = "#88C0D0"
TEAL = "#8FBCBB"
BLUE = "#81A1C1"
MUTED = "#5E81AC"
DIM = "#4C5A68"
RED = "#BF616A"
GREEN = "#A3BE8C"
YELLOW = "#EBCB8B"
PURPLE = "#B48EAD"

THEME = Theme(
    {
        "monet": FG,
        "monet.title": f"bold {TEAL}",
        "monet.accent": TEAL,
        "monet.blue": BLUE,
        "monet.muted": MUTED,
        "monet.dim": DIM,
        "monet.ok": GREEN,
        "monet.warn": YELLOW,
        "monet.err": RED,
        "monet.key": f"bold {BLUE}",
        "table.header": f"bold {BLUE}",
        "table.border": TEAL,
        "table.title": f"bold {TEAL}",
        "rule": DIM,
    }
)

console = Console(theme=THEME, highlight=False, emoji=False)


def banner() -> None:
    """Print the compact wordmark used by CLI commands."""
    console.print()
    console.print(
        f"  [bold {TEAL}]◆ MONET CODE[/]  [monet.dim]terminal atelier for local models[/]",
        highlight=False,
    )


def swatches() -> None:
    """Print the palette as colored swatches."""
    rows = [
        ("canvas", BG, "background"),
        ("frost blue", FG, "primary text"),
        ("glacier teal", TEAL, "primary accent"),
        ("arctic blue", BLUE, "secondary accent"),
        ("polar night", DIM, "dim chrome"),
    ]
    banner()
    console.print()
    for name, color, role in rows:
        console.print(
            f"  [on {color}]      [/]  [bold {FG}]{name:<13}[/] [monet.dim]{color:<9} {role}[/]"
        )
    console.print()


def rarity_color(value: str) -> str:
    return TEAL if value else DIM
