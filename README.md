# ◆ Monet Code

A local-first AI coding agent for the terminal. Monet Code pairs a Go/Charm
TUI with a Python/Typer CLI: the TUI is for conversation, editing and browsing
your project; the CLI is for scripting, model management and tidying folders.
Everything talks to your own [Ollama](https://ollama.com) server and writes to
one shared config file.

```
  ◆ MONET CODE  v0.1.0  ·  local agent                                   ● ollama 0.40.16  qwen3-coder:30b
  Chat   Editor   Models   Files                                       F2‑5 views  ·  ⌃o files  ·  F1 help
 FILES             monet │ FILE ORGANIZATION                                 ~/projects/monet  ·  6 loose files
──────────────────────────────│╭ BY CATEGORY ────────────────────────────────────────────────────────────╮
 ▸ cmd                        ││  Documents .....  1 files     12.4 KB  →  Documents/                     │
 ▸ internal                   ││  Images ........  2 files      4.1 MB  →  Images/                        │
 · go.mod                     │╰─────────────────────────────────────────────────────────────────────────╯
 · go.sum                     │╭ PREVIEW ────────────────────────────────────────────────────────────────╮
 · README.md                  ││  notes.md  →  Documents/notes.md                                         │
 · main.py                    ││  shot.png  →  Images/shot.png                                            │
                              │╰─────────────────────────────────────────────────────────────────────────╯
 o preview  ·  y apply  ·  r rescan  ·  n file  ·  N folder                              ~/projects/monet
```

## Features

- **Chat** — streaming answers with thinking display, Markdown rendering and a
  native Ollama tool loop. The agent can read, write, list, search and create
  directories; writes always ask for approval first.
- **Editor** — a real text editor with line numbers, syntax-neutral theming,
  dirty tracking and `⌃s` save. Open files from the tree or with `/edit`.
- **Model catalog** — the installed Ollama models next to a curated catalog of
  coding models, with size, parameters, family and purpose. Pull or delete
  models without leaving the TUI, and the active model persists.
- **File organization** — preview a dry-run plan that sorts loose top-level
  files into `Documents/`, `Images/`, `Code/` and friends, then apply it. Hidden
  files, symlinks and folders are never touched.
- **File tree** — a keyboard-driven tree with create, rename and delete.
- **Shared config** — the TUI and CLI both use
  `~/.config/monet/config.json` (or `$MONET_CONFIG_DIR/config.json`), so
  `monet models use` changes the TUI's next start.

## Requirements

| Tool | Version | Notes |
| --- | --- | --- |
| Go | 1.24+ | builds `bin/monet-tui` |
| Python | 3.9+ | runs the `monet` CLI |
| Ollama | any recent | `ollama serve`, with at least one model |

## Install

One line, no sudo, nothing installed system-wide:

```bash
curl -fsSL https://raw.githubusercontent.com/SYOP200/monet-code/main/install.sh | bash
```

The installer checks Go and Python, installs the sources under
`~/.local/share/monet-code`, builds the TUI, creates a virtualenv for the CLI,
and writes a `monet` launcher to `~/.local/bin`. Your config in
`~/.config/monet` is left untouched, so an upgrade keeps the active model and
settings.

Useful flags (pass them after `bash -s --`):

```bash
# choose locations, or add ~/.local/bin to PATH automatically
curl -fsSL .../install.sh | bash -s -- --dir ~/opt/monet --bin-dir ~/.local/bin --modify-path

# install a tag or branch, or replace an existing install
curl -fsSL .../install.sh | bash -s -- --ref v0.1.0 --force

# remove the launcher, install tree and PATH block (config is kept)
curl -fsSL .../install.sh | bash -s -- --uninstall
```

The full set is `--dir`, `--bin-dir`, `--ref`, `--repo`, `--source`,
`--modify-path`, `--force`, `--uninstall` and `--help`. The same values can be
set with `MONET_HOME`, `MONET_BIN_DIR`, `MONET_REF` and `MONET_REPO`.

If `~/.local/bin` is not on your `PATH`, the installer prints the export line to
add (or re-run it with `--modify-path`).

### From a checkout

```bash
git clone https://github.com/SYOP200/monet-code.git
cd monet-code
make setup      # creates .venv, installs the CLI, downloads Go modules
make build      # builds the TUI and syncs the catalog
```

Then launch either front end:

```bash
source .venv/bin/activate
monet                 # opens the TUI in the current directory
monet tui ~/code/app  # or point it at a project
monet --help
```

`monet tui` builds the Go binary automatically if it is stale. Set
`MONET_TUI_BIN` to use a binary installed elsewhere.

To keep the CLI available without activating the venv, install it with `pipx`:

```bash
pipx install --editable .
```

## Using the TUI

| Key | Action |
| --- | --- |
| `F2`..`F5` / `⌃1`..`⌃4` | switch between Chat, Editor, Models and Files |
| `⌃o` | focus the file tree (`esc` leaves it) |
| `F1` / `⌃h` | help |
| `enter` | send a message, open a file, activate a model |
| `alt+enter` | newline in the composer or editor |
| `pgup` / `pgdn` | scroll the conversation or preview |
| `⌃s` | save the open file |
| `⌃n` | new file (editor) |
| `esc` | cancel a turn, close a dialog, go back to chat |
| `⌃c` | cancel the running turn, or quit when idle |

In the file tree: `a` new file, `A` new folder, `R` rename, `d` delete,
`r` refresh. In Models: `tab` switches Installed/Catalog, `p` pulls a model,
`d` deletes one, `/` filters. In Files: `o` builds the preview, `y` applies it.

When the agent wants to write a file you will see an approval box. Press `y` to
allow the single call, or `n` to deny it and let the model continue.

### Slash commands

```
/help                 show help
/model [name]         show models, or switch to name
/files                open the organizer view
/edit [path]          open a file in the editor
/new <path>           create and open a file
/save                 save the open file
/clear                reset the conversation
/status               model, Ollama version, history size, workspace
/tools                list the agent tools
/palette              print the color palette
/quit                 quit
```

## Using the CLI

```bash
# one-shot questions (streams the answer)
monet ask "explain the error handling in internal/agent" -m qwen3.5:4b
monet ask "summarize this module" --file internal/files/files.go --json

# models
monet models list --all
monet models recommend
monet models pull qwen2.5-coder:7b
monet models use qwen2.5-coder:7b
monet models show qwen3-coder:30b
monet models ps
monet models rm old-model:tag

# files
monet files tree . --depth 3
monet files stats .
monet files organize .            # dry run, prints the plan
monet files organize . --apply    # asks for confirmation, then moves
monet files find "TODO" --content
monet files find config

# shared config
monet config show
monet config set editor_tab_size 2
monet config path

# diagnostics
monet doctor
monet palette
```

## Configuration

The config file is shared by both front ends:

```json
{
  "model": "qwen3-coder:30b",
  "recent_models": ["qwen3-coder:30b", "qwen3.5:4b"],
  "recent_directories": ["/Users/you/projects/monet"],
  "editor_tab_size": 4,
  "ollama_url": "",
  "system_prompt": "",
  "show_thinking": true,
  "organize_folders": true
}
```

- `model` — the active Ollama model. Changing it in the TUI writes this file;
  `monet models use` does the same.
- `ollama_url` — leave empty to use `$OLLAMA_HOST`, then
  `http://127.0.0.1:11434`.
- `editor_tab_size` — spaces inserted by `tab` in the editor.
- `system_prompt` — appended instructions for `monet ask`.

Set `MONET_CONFIG_DIR` to keep multiple profiles, for example:

```bash
MONET_CONFIG_DIR=~/.config/monet/work monet tui ~/code/work
```

## Model catalog

The catalog is authored once in
[`internal/catalog/models.json`](internal/catalog/models.json) and embedded in
the Go binary. `make sync-catalog` copies it to `monet_cli/models.json` for the
CLI, and a test fails if the two copies drift.

Currently curated: Qwen 3 Coder, Qwen 3.6, Qwen 3.5, Qwen 2.5 Coder,
DeepSeek R1, DeepSeek Coder V2, Dolphin 3, Gemma 3, Llama 3.3, Mistral, Phi 4,
CodeLlama, StarCoder 2 and Nomic Embed. Installed models outside the catalog
still appear in the Installed list and can be selected.

When no model is configured, the TUI picks the first curated model that is
installed (falling back to the first installed model), so a fresh start works
without configuration.

## Agent tools

| Tool | Approval | Purpose |
| --- | --- | --- |
| `read_file` | no | read a text file with line numbers and paging |
| `list_files` | no | list a directory tree up to depth 3 |
| `search_files` | no | search file contents |
| `write_file` | **yes** | write the complete new contents of a file |
| `create_directory` | **yes** | create a directory |

Every path is resolved with `files.SafeJoin`, which rejects anything that
escapes the workspace, including `..` traversal and absolute paths outside the
root. Writes are capped at 2 MB; reads at 2 MB per call.

## Architecture

```
cmd/monet-tui/         Go entry point (flags: --dir, --model, --ollama-url)
internal/theme/        palette and lipgloss styles
internal/config/       shared config load/save
internal/catalog/      embedded curated catalog
internal/ollama/       native Ollama client (streaming chat, pull, tools)
internal/files/        organizer, safe paths, text IO, search
internal/agent/        tool executor and streaming agent loop
internal/ui/           bubbletea app: chat, editor, models, files, sidebar
monet_cli/             Typer + Rich CLI, Ollama client, organizer, catalog
tests/                 Python unittest suite
```

The TUI uses [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss). The CLI uses Typer and
Rich. Markdown in the chat is rendered by a small purpose-built renderer so the
frost palette stays exact at every width.

## Palette

| Role | Color |
| --- | --- |
| Canvas | `#0D1117` |
| Frost blue (primary text) | `#88C0D0` |
| Glacier teal (accent) | `#8FBCBB` |
| Arctic blue (secondary accent) | `#81A1C1` |

Run `monet palette` to see swatches, or `/palette` in the TUI.

## Development

```bash
make test     # go test ./... + Python unittest
make fmt      # gofmt + compileall
make vet      # go vet ./...
make build    # rebuild both front ends
make clean    # remove bin/, .venv and caches
```

Go tests cover the organizer (dry-run safety, collisions), path escapes, tool
execution, config round-trips, the embedded catalog and the exact frame size of
every TUI view. The Python suite mirrors the organizer and config behavior.

To drive the TUI against a live server in a test, use the snapshot mode:

```bash
./bin/monet-tui --snapshot --width 120 --height 40 --dir .
./bin/monet-tui --snapshot --snapshot-tab models --width 120 --height 40
./bin/monet-tui --snapshot --snapshot-tab editor --snapshot-file README.md
```

## Troubleshooting

**"cannot reach Ollama"** — start the server with `ollama serve`, then check
`monet doctor`. If it listens elsewhere, set `ollama_url` or `OLLAMA_HOST`.

**A model ignores tools** — not every model supports native tool calling.
Qwen 3 Coder and Qwen 2.5 Coder are the most reliable choices; run
`monet models recommend`.

**`monet tui` asks for Go** — the CLI builds the TUI from source when it is
missing or stale. Install Go, or point `MONET_TUI_BIN` at an existing binary.

**Colors look wrong** — use a truecolor terminal. The TUI paints every cell
with the canvas color, so partial 256-color support will look muddy.

## License

MIT.
