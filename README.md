# ◆ Monet Code

A local-first AI coding agent for the terminal. Monet Code pairs a Go/Charm
TUI with a Python/Typer CLI: the TUI is for conversation, editing and browsing
your project; the CLI is for scripting, model management and tidying folders.
Everything talks to your own [Ollama](https://ollama.com) server and writes to
one shared config file.

```
__╱╲╲╲╲____________╱╲╲╲╲_______╱╲╲╲╲╲_______╱╲╲╲╲╲_____╱╲╲╲__╱╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲__╱╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲_        
 _╲╱╲╲╲╲╲╲________╱╲╲╲╲╲╲_____╱╲╲╲╱╱╱╲╲╲____╲╱╲╲╲╲╲╲___╲╱╲╲╲_╲╱╲╲╲╱╱╱╱╱╱╱╱╱╱╱__╲╱╱╱╱╱╱╱╲╲╲╱╱╱╱╱__       
  _╲╱╲╲╲╱╱╲╲╲____╱╲╲╲╱╱╲╲╲___╱╲╲╲╱__╲╱╱╱╲╲╲__╲╱╲╲╲╱╲╲╲__╲╱╲╲╲_╲╱╲╲╲___________________╲╱╲╲╲_______      
   _╲╱╲╲╲╲╱╱╱╲╲╲╱╲╲╲╱_╲╱╲╲╲__╱╲╲╲______╲╱╱╲╲╲_╲╱╲╲╲╱╱╲╲╲_╲╱╲╲╲_╲╱╲╲╲╲╲╲╲╲╲╲╲___________╲╱╲╲╲_______     
    _╲╱╲╲╲__╲╱╱╱╲╲╲╱___╲╱╲╲╲_╲╱╲╲╲_______╲╱╲╲╲_╲╱╲╲╲╲╱╱╲╲╲╲╱╲╲╲_╲╱╲╲╲╱╱╱╱╱╱╱____________╲╱╲╲╲_______    
     _╲╱╲╲╲____╲╱╱╱_____╲╱╲╲╲_╲╱╱╲╲╲______╱╲╲╲__╲╱╲╲╲_╲╱╱╲╲╲╱╲╲╲_╲╱╲╲╲___________________╲╱╲╲╲_______   
      _╲╱╲╲╲_____________╲╱╲╲╲__╲╱╱╱╲╲╲__╱╲╲╲____╲╱╲╲╲__╲╱╱╲╲╲╲╲╲_╲╱╲╲╲___________________╲╱╲╲╲_______  
       _╲╱╲╲╲_____________╲╱╲╲╲____╲╱╱╱╲╲╲╲╲╱_____╲╱╲╲╲___╲╱╱╲╲╲╲╲_╲╱╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲_______╲╱╲╲╲_______ 
        _╲╱╱╱______________╲╱╱╱_______╲╱╱╱╱╱_______╲╱╱╱_____╲╱╱╱╱╱__╲╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱________╲╱╱╱________
_____________________________________╱╲╲╲_________________                                              
 ____________________________________╲╱╲╲╲_________________                                             
  ____________________________________╲╱╲╲╲_________________                                            
   _____╱╲╲╲╲╲╲╲╲_____╱╲╲╲╲╲___________╲╱╲╲╲______╱╲╲╲╲╲╲╲╲__                                           
    ___╱╲╲╲╱╱╱╱╱╱____╱╲╲╲╱╱╱╲╲╲____╱╲╲╲╲╲╲╲╲╲____╱╲╲╲╱╱╱╱╱╲╲╲_                                          
     __╱╲╲╲__________╱╲╲╲__╲╱╱╲╲╲__╱╲╲╲╱╱╱╱╲╲╲___╱╲╲╲╲╲╲╲╲╲╲╲__                                         
      _╲╱╱╲╲╲________╲╱╱╲╲╲__╱╲╲╲__╲╱╲╲╲__╲╱╲╲╲__╲╱╱╲╲╱╱╱╱╱╱╱___                                        
       __╲╱╱╱╲╲╲╲╲╲╲╲__╲╱╱╱╲╲╲╲╲╱___╲╱╱╲╲╲╲╲╲╲╱╲╲__╲╱╱╲╲╲╲╲╲╲╲╲╲_                                       
        ____╲╱╱╱╱╱╱╱╱_____╲╱╱╱╱╱______╲╱╱╱╱╱╱╱╲╱╱____╲╱╱╱╱╱╱╱╱╱╱__                                      
```

## Features

- **Chat** — streaming answers with a persistent reasoning trace, Markdown
  rendering and a native Ollama tool loop. Click a `thinking` line (or run
  `/thinking`) to read the model's full reasoning in a scrollable popup.
- **Slash-command palette** — type `/` to open a filtered command list above the
  composer. `↑↓` choose, `tab` completes, `enter` runs, `esc` closes; every row
  is clickable. Commands are discovered from disk as reusable skills.
- **Editor** — a real text editor with line numbers, syntax-neutral theming,
  dirty tracking and `⌃s` save. Open files from the tree or with `/editor`.
- **Model catalog** — installed Ollama models next to a curated catalog of coding
  models, with size, parameters, family and purpose. Pull or delete models
  without leaving the TUI, and the active model persists.
- **File organization** — preview a dry-run plan that sorts loose top-level files
  into `Documents/`, `Images/`, `Code/` and friends, then apply it. Hidden files,
  symlinks and folders are never touched.
- **File tree and worktrees** — a mouse- and keyboard-driven tree with create,
  rename and delete, plus git worktree creation for parallel branches.
- **Safe agent tools** — the model can read, write, create directories, delete
  single files, move files, run builds/tests, and inspect or commit git state.
  Mutations require approval, every edit is checkpointed, and destructive shell
  patterns such as `rm -rf`, `sudo`, `mkfs` and `git clean` are refused.
- **Sessions and rewind** — conversations are saved automatically, `/resume`
  restores the transcript, `/compact` summarizes context, and `/rewind` restores
  a checkpoint taken before the agent's edit.
- **Project memory** — `AGENTS.md`, `CLAUDE.md`, `MONET.md` and
  `.monet/memory.md` are loaded into the system prompt. `/init` creates an
  `AGENTS.md`, `/remember` appends to memory, and `/reload` re-reads it.
- **Hooks and subagents** — shell hooks run after edits and before commits;
  read-only subagents investigate in parallel; sessions can message each other.
- **Shared config** — the TUI and CLI both use
  `~/.config/monet/config.json` (or `$MONET_CONFIG_DIR/config.json`), so
  `monet models use` changes the TUI's next start.

## Requirements

| Tool | Version | Notes |
| --- | --- | --- |
| curl + tar | any | only needed by the installer |
| Python | 3.9+ | runs the `monet` CLI; optional, the TUI is a standalone binary |
| Ollama | any recent | `ollama serve`, with at least one model |
| Git | optional | enables diff, commit, branches, push and worktrees |
| GitHub CLI | optional | enables `/pr` and `/prs` |
| Go | optional | only to build from source; releases ship prebuilt binaries |

## Install

One line, no sudo, nothing installed system-wide. Prebuilt binaries are
downloaded for macOS and Linux (amd64 and arm64), so **Go, Git, pip and
virtualenvs are not required**:

```bash
curl -fsSL https://raw.githubusercontent.com/SYOP200/monet-code/main/install.sh | bash
```

The installer downloads the latest GitHub release, verifies every asset against
the published SHA-256 checksums, installs the static TUI and a self-contained
CLI under `~/.local/share/monet-code`, and writes a `monet` launcher to
`~/.local/bin`. Your config in `~/.config/monet` is left untouched, so an
upgrade keeps the active model and settings.

This works the same on Arch, Ubuntu, Fedora and macOS; there is nothing to
install with pacman/apt first except `curl`, which both platforms ship.

Useful flags (pass them after `bash -s --`):

```bash
# choose locations, or add ~/.local/bin to PATH automatically
curl -fsSL .../install.sh | bash -s -- --dir ~/opt/monet --bin-dir ~/.local/bin --modify-path

# pin a release, or replace an existing install
curl -fsSL .../install.sh | bash -s -- --ref v0.2.1 --force

# build a branch from source instead (needs Go 1.24+)
curl -fsSL .../install.sh | bash -s -- --ref main

# remove the launcher, install tree and PATH block (config is kept)
curl -fsSL .../install.sh | bash -s -- --uninstall
```

The full set is `--dir`, `--bin-dir`, `--ref`, `--repo`, `--source`,
`--modify-path`, `--force`, `--uninstall` and `--help`. The same values can be
set with `MONET_HOME`, `MONET_BIN_DIR`, `MONET_REF` and `MONET_REPO`.

If `~/.local/bin` is not on your `PATH`, the installer prints the export line to
add (or re-run it with `--modify-path`).

### Updating

Prebuilt installs update in place; there is no need to re-run the installer:

```bash
monet update             # download and install the latest release
monet update --check     # only report whether a newer release exists
monet update --force     # reinstall the current release
monet update --tag v0.2.1
```

Installs made before v0.2.1 do not ship the updater yet. Run the installer once
with `--force` to reach v0.2.1; every later release can then be installed with
`monet update` or `/update`.

Every asset is checked against the release's SHA-256 sums before it replaces
`bin/monet-tui` or the bundled CLI. Config, sessions and checkpoints are
untouched. Restart the TUI afterwards to run the new binary.

From inside the TUI, `/update` does the same after a confirmation, `/update check`
only reports whether a newer release exists, and `/update force` reinstalls the
current one. The running process keeps using the old binary until you restart,
so an update never interrupts a turn.

### From a checkout

Only needed for development; the normal install does not use Go:

```bash
git clone https://github.com/SYOP200/monet-code.git
cd monet-code
make setup      # creates .venv, installs the CLI, downloads Go modules
make release    # builds dist/ assets for all four platforms
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
| mouse | click tabs, command, model, file and preview rows, thinking lines |
| `F2`..`F5` / `⌃1`..`⌃4` | switch between Chat, Editor, Models and Files |
| `⌃o` | focus the file tree (`esc` leaves it) |
| `F1` | help, including every command |
| `enter` | send a message, run a command, open a file, activate a model |
| `alt+enter` | newline in the composer or editor |
| `pgup` / `pgdn` / wheel | scroll the conversation or preview |
| `⌃s` | save the open file |
| `⌃n` | new file (editor) |
| `esc` | close the popup, cancel a turn, go back to chat |
| `⌃c` | cancel the running turn and show the exit hint |
| `⌃d` / `:q` | quit (`:q!`, `:wq`, `:x` also work) |

In the file tree: `a` new file, `A` new folder, `R` rename, `d` delete,
`r` refresh. In Models: `tab` switches Installed/Catalog, `p` pulls a model,
`d` deletes one, `/` filters. In Files: `o` builds the preview, `y` applies it.

When the agent wants to write, delete, move, run a command or commit, an
approval box appears. Press `y` to allow that single call or `n` to deny it and
let the model continue. Scroll never escapes the transcript; only the exit keys
leave the TUI.

### Slash commands

```
/help /? /h               help
/clear                    reset the conversation and start a new session
/compact                  summarize older turns to free context
/resume [session]         restore the latest or named session
/sessions                 list saved sessions
/session /id              show this session id, workspace and model
/checkpoint [label]       snapshot the open file before a risky change
/rewind [n] /undo         restore the nth most recent checkpoint
/init                     create AGENTS.md project memory
/remember <note>          append a note to .monet/memory.md
/memory                   show the memory files that are loaded
/reload                   re-read skills, memory, hooks and the tree

/model [name]             switch model, or open the catalog
/models /files /editor    switch views
/chat /new /save          composer helpers
/review [path]            ask the agent to review the working tree
/diff [path] /git <sub>   show git state (status, diff, log, branch…)
/commit <message>         stage and commit after confirmation
/worktree <name>          create a sibling git worktree
/worktrees                list worktrees
/branch <name>            create and switch to a branch
/push [branch]            push (never forced)
/pr [title] /prs          open or list GitHub pull requests via gh

/agent <task>             start a read-only subagent
/agents                   show subagent status
/msg <session> <text>     message another Monet session
/inbox                    read messages sent to this session
/hooks                    show configured hooks
/skills                   list custom commands found on disk
/thinking /think          open the last reasoning trace
/status /tools /palette   diagnostics
/update [check|force|vX]  upgrade the installed TUI and CLI in place
/quit /exit /q            quit
```

### Custom skills

Drop Markdown files into one of these directories and they appear in the `/`
palette after `/reload`:

```
.monet/commands/<name>.md          project skills
.claude/commands/<name>.md        Claude Code-compatible skills
~/.config/monet/commands/<name>.md user-wide skills
```

Optional front matter sets the description. `$ARGUMENTS` (or `{{args}}`) is
replaced with whatever follows the command; otherwise the arguments are appended
to the prompt.

```markdown
---
description: review a pull request
---
Review $ARGUMENTS as a senior engineer. Use git_diff first, then read the
changed files. Report bugs, risky changes and missing tests.
```

Project skills win over user skills with the same name; built-ins win over both.

## Project memory and hooks

The system prompt is rebuilt from these files in order when a session starts,
after `/remember`, `/init` or `/reload`:

```
AGENTS.md
CLAUDE.md
MONET.md
.monet/memory.md
```

Keep them short and specific: conventions, commands, architecture and things to
avoid. `/remember` appends a dated line to `.monet/memory.md`.

Hooks live in `.monet/hooks.json` and run with a 30-second timeout:

```json
{
  "after_edit": "gofmt -w \"$MONET_FILE\"",
  "before_commit": "go test ./...",
  "after_tool": "echo $MONET_TOOL >> .monet/tool.log"
}
```

`after_edit` receives `$MONET_FILE`; `after_tool` receives `$MONET_TOOL` and,
for commands, `$MONET_COMMAND`. Hook failures are reported back to the model but
never abort an edit. A failing `before_commit` hook does block the commit.

## Sessions, checkpoints and messaging

The TUI saves the conversation after every turn under
`~/.config/monet/sessions/<id>.json` (thinking included). `/resume` loads the
latest transcript and model; `/sessions` lists IDs you can pass to `/resume` or
`/msg`. `/compact` replaces older turns with a model-written briefing.

Before `write_file`, `delete_file` and `move_file`, the agent writes the old
contents (or records that the file did not exist) under `.monet/checkpoints`.
`/rewind` restores the most recent one, `/rewind 2` the one before it, and
`/checkpoint` snapshots the file currently open in the editor.

Every session has an ID and a JSONL inbox. `/msg latest please check the API`
delivers a note to the most recent other session; `/inbox` reads and clears it.

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
monet update              # upgrade to the latest release
monet update --check
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
- `system_prompt` — extra instructions appended to the agent's system prompt.

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
| `git_status` | no | short status and branch |
| `git_diff` | no | working-tree diff, optionally for one path |
| `git_log` | no | recent one-line commits |
| `write_file` | **yes** | write complete new contents; checkpointed first |
| `create_directory` | **yes** | create a directory (and parents) |
| `delete_file` | **yes** | delete one regular file; directories refused |
| `move_file` | **yes** | move/rename one file; overwrite refused |
| `run_command` | **yes** | run a shell command in the workspace |
| `git_commit` | **yes** | run `before_commit`, stage, and commit |

Every path is resolved with `files.SafeJoin`, which rejects anything that
escapes the workspace, including `..` traversal and absolute paths outside the
root. `.git` and `.monet` are never writable by the model. Writes are capped at
4 MB; reads and search results are capped per call.

`run_command` executes with `bash -lc` inside a workspace subdirectory and a
hard timeout (default 60 s, max 5 min). Before execution, destructive patterns
are rejected: recursive `rm`, `sudo`, `mkfs`, `dd`, broad `pkill`/`killall`,
recursive `chmod`/`chown`, `git clean`, `git reset --hard`, forced pushes,
`find -delete`, and disk redirection. Output is capped at 256 KB.

Read-only subagents get only the six inspection tools, cannot ask for approval,
and run with a shorter step budget. Up to four can run at once; `/agents` shows
their status and findings return to the transcript.

## Architecture

```
cmd/monet-tui/         Go entry point (flags: --dir, --model, --ollama-url)
internal/theme/        palette and lipgloss styles (black canvas)
internal/config/       shared config load/save
internal/catalog/      embedded curated catalog
internal/ollama/       native Ollama client (streaming chat, pull, tools)
internal/files/        organizer, safe paths, text IO, search, checkpoints
internal/agent/        tool executor, shell guard, hooks, git, agent loop
internal/session/      saved conversations and cross-session inboxes
internal/ui/           bubbletea app: chat, editor, models, files, sidebar,
                       commands, mouse hit-testing, thinking modal
monet_cli/             Typer + Rich CLI, Ollama client, organizer, catalog
tests/                 Python unittest suite
```

The TUI uses [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss), with mouse cell-motion
enabled. The CLI uses Typer and Rich. Markdown in the chat is rendered by a small
purpose-built renderer so the frost palette stays exact at every width.

## Palette

| Role | Color |
| --- | --- |
| Canvas (full terminal) | `#000000` |
| Raised rows (chips, selection) | `#151515` |
| Frost blue (primary text) | `#88C0D0` |
| Glacier teal (accent) | `#8FBCBB` |
| Arctic blue (secondary accent) | `#81A1C1` |

The chrome, sidebar, transcript and modals all paint the canvas color, so no
terminal background shows through. Run `monet palette` to see swatches, or
`/palette` in the TUI.

## Development

```bash
make test     # go test ./... + Python unittest
make fmt      # gofmt + compileall
make vet      # go vet ./...
make build    # rebuild both front ends
make clean    # remove bin/, .venv and caches
```

Go tests cover the organizer, path escapes, checkpoint restore, shell refusal
rules, tool execution, hooks, git helpers, sessions, messaging, custom command
loading, slash filtering, mouse tab clicks, the exit guard and the exact frame
size of every view and modal. The Python suite mirrors the organizer and config
behavior.

To drive the TUI against a live server in a test, use the snapshot mode:

```bash
./bin/monet-tui --snapshot --width 120 --height 40 --dir .
./bin/monet-tui --snapshot --snapshot-tab models --width 120 --height 40
./bin/monet-tui --snapshot --snapshot-tab editor --snapshot-file README.md
./bin/monet-tui --snapshot --snapshot-tab thinking --width 100 --height 30
```

## Troubleshooting

**"cannot reach Ollama"** — start the server with `ollama serve`, then check
`monet doctor`. If it listens elsewhere, set `ollama_url` or `OLLAMA_HOST`.

**A model ignores tools** — not every model supports native tool calling.
Qwen 3 Coder and Qwen 2.5 Coder are the most reliable choices; run
`monet models recommend`.

**A command was refused** — the safety guard blocks recursive deletion,
privilege escalation and disk tools. Ask the agent to delete files one at a time,
or run the command yourself outside Monet.

**A bad edit landed** — run `/rewind` to restore the last checkpoint, or
`/rewind 2` for the one before it.

**`monet tui` asks for Go** — a prebuilt install never needs Go. If you see
this, the install is from source; re-run the installer (which downloads a
release), or point `MONET_TUI_BIN` at an existing binary.

**Colors look wrong** — use a truecolor terminal. The TUI paints every cell
with the canvas color, so partial 256-color support will look muddy.

## License

MIT.
