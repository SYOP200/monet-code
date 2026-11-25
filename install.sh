#!/usr/bin/env bash
#
# Monet Code installer
#   curl -fsSL https://raw.githubusercontent.com/SYOP200/monet-code/main/install.sh | bash
#
# Installs the Go TUI and the Python/Typer CLI side by side, then adds a
# `monet` shim so both stay in sync. Nothing is installed system-wide.
#
# Options (pass after `bash -s --`):
#   --dir PATH        install location    (default: ~/.local/share/monet-code)
#   --bin-dir PATH    shim location       (default: ~/.local/bin)
#   --ref REF         git branch or tag   (default: main)
#   --modify-path     append BIN_DIR to your shell rc file
#   --force           replace an existing install
#   --uninstall       remove Monet Code
#   --source PATH     install from a local checkout (development)
#   -h, --help        show this help
#
# Environment:
#   MONET_HOME, MONET_BIN_DIR, MONET_REF, MONET_REPO,
#   MONET_INSTALL_SOURCE, MONET_TUI_BIN, NO_COLOR
#
set -euo pipefail

REPO="${MONET_REPO:-SYOP200/monet-code}"
REF="${MONET_REF:-main}"
DEFAULT_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/monet-code"
INSTALL_DIR="${MONET_HOME:-$DEFAULT_HOME}"
BIN_DIR="${MONET_BIN_DIR:-$HOME/.local/bin}"
SOURCE="${MONET_INSTALL_SOURCE:-}"

MODIFY_PATH=0
FORCE=0
UNINSTALL=0
MARKER=".monet-code-install"

# --------------------------------------------------------------------------- #
# output
# --------------------------------------------------------------------------- #

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-dumb}" != "dumb" ]; then
	BOLD=$'\033[1m'
	DIM=$'\033[2m'
	TEAL=$'\033[38;2;143;188;187m'
	BLUE=$'\033[38;2;129;161;193m'
	FROST=$'\033[38;2;136;192;208m'
	GREEN=$'\033[38;2;163;190;140m'
	YELLOW=$'\033[38;2;235;203;139m'
	RED=$'\033[38;2;191;97;106m'
	RESET=$'\033[0m'
else
	BOLD="" DIM="" TEAL="" BLUE="" FROST="" GREEN="" YELLOW="" RED="" RESET=""
fi

say() { printf '%s\n' "$*"; }
step() { printf '%s\n' "  ${TEAL}◆${RESET} $*"; }
ok() { printf '%s\n' "  ${GREEN}✓${RESET} $*"; }
warn() { printf '%s\n' "  ${YELLOW}!${RESET} $*" >&2; }
die() {
	printf '%s\n' "  ${RED}✗${RESET} $*" >&2
	exit 1
}
have() { command -v "$1" >/dev/null 2>&1; }

usage() {
	cat <<'EOF'
Monet Code installer

Usage:
  curl -fsSL https://raw.githubusercontent.com/SYOP200/monet-code/main/install.sh | bash
  curl -fsSL .../install.sh | bash -s -- [options]

Options:
  --dir PATH       install location (default: ~/.local/share/monet-code)
  --bin-dir PATH   where to write the monet shim (default: ~/.local/bin)
  --ref REF        git branch or tag to install (default: main)
  --modify-path    append the bin directory to your shell rc file
  --force          replace an existing install
  --uninstall      remove Monet Code from this machine
  --source PATH    install from a local checkout instead of GitHub
  -h, --help       show this help

Environment:
  MONET_HOME, MONET_BIN_DIR, MONET_REF, MONET_REPO,
  MONET_INSTALL_SOURCE, MONET_TUI_BIN, NO_COLOR

After installing:
  monet              open the TUI
  monet ask "hi"     one-shot prompt
  monet doctor       check Go, Python, Ollama and the config
EOF
}

# --------------------------------------------------------------------------- #
# arguments
# --------------------------------------------------------------------------- #

while [ $# -gt 0 ]; do
	case "$1" in
	--dir | --home)
		[ $# -ge 2 ] || die "$1 needs a path"
		INSTALL_DIR="$2"
		shift 2
		;;
	--bin-dir)
		[ $# -ge 2 ] || die "$1 needs a path"
		BIN_DIR="$2"
		shift 2
		;;
	--ref)
		[ $# -ge 2 ] || die "$1 needs a branch or tag"
		REF="$2"
		shift 2
		;;
	--repo)
		[ $# -ge 2 ] || die "$1 needs owner/repo"
		REPO="$2"
		shift 2
		;;
	--source)
		[ $# -ge 2 ] || die "$1 needs a path"
		SOURCE="$2"
		shift 2
		;;
	--modify-path | --path)
		MODIFY_PATH=1
		shift
		;;
	--force)
		FORCE=1
		shift
		;;
	--uninstall)
		UNINSTALL=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		die "unknown option: $1 (try --help)"
		;;
	esac
done

INSTALL_DIR="${INSTALL_DIR%/}"
BIN_DIR="${BIN_DIR%/}"

# --------------------------------------------------------------------------- #
# uninstall
# --------------------------------------------------------------------------- #

remove_path_block() {
	local rc="$1"
	[ -f "$rc" ] || return 0
	grep -q "# >>> monet code >>>" "$rc" || return 0
	local tmp
	tmp="$(mktemp "${TMPDIR:-/tmp}/monet-rc.XXXXXX")"
	awk '
		/^# >>> monet code >>>$/ { skip = 1; next }
		/^# <<< monet code <<<$/ { skip = 0; next }
		skip != 1 { print }
	' "$rc" >"$tmp"
	mv "$tmp" "$rc"
	ok "removed the PATH block from $rc"
}

if [ "$UNINSTALL" = 1 ]; then
	say ""
	say "  ${BOLD}◆ MONET CODE${RESET} ${DIM}uninstall${RESET}"
	say ""
	removed=0
	if [ -f "$BIN_DIR/monet" ] && grep -q "# monet-code installer" "$BIN_DIR/monet" 2>/dev/null; then
		rm -f "$BIN_DIR/monet"
		ok "removed $BIN_DIR/monet"
		removed=1
	fi
	if [ -L "$BIN_DIR/monet-tui" ]; then
		rm -f "$BIN_DIR/monet-tui"
		ok "removed $BIN_DIR/monet-tui"
		removed=1
	fi
	if [ -d "$INSTALL_DIR" ] && [ -f "$INSTALL_DIR/$MARKER" ]; then
		rm -rf "$INSTALL_DIR"
		ok "removed $INSTALL_DIR"
		removed=1
	elif [ -d "$INSTALL_DIR" ]; then
		warn "$INSTALL_DIR is not a Monet Code install; leaving it alone"
	fi
	remove_path_block "${ZDOTDIR:-$HOME}/.zshrc"
	remove_path_block "$HOME/.bashrc"
	remove_path_block "$HOME/.bash_profile"
	if [ "$removed" = 0 ]; then
		warn "nothing to remove"
	else
		say ""
		ok "Monet Code was uninstalled. Your config in ~/.config/monet was kept."
	fi
	say ""
	exit 0
fi

# --------------------------------------------------------------------------- #
# dependency checks
# --------------------------------------------------------------------------- #

say ""
say "  ${BOLD}◆ MONET CODE${RESET} ${DIM}a local-first coding agent for the terminal${RESET}"
say ""

step "checking dependencies"

case "$(uname -s)" in
Darwin | Linux) ;;
*) die "unsupported platform: $(uname -s). Monet Code targets macOS and Linux." ;;
esac

have git || die "git is required. Install git and try again."
[ -n "$SOURCE" ] || have curl || die "curl is required. Install curl and try again."

if have python3; then
	PYTHON=python3
elif have python; then
	PYTHON=python
else
	die "Python 3.9+ is required. https://www.python.org/downloads/"
fi
"$PYTHON" - <<'PY' || die "Python 3.9 or newer is required."
import sys
raise SystemExit(0 if sys.version_info >= (3, 9) else 1)
PY
PY_VERSION="$("$PYTHON" -c 'import platform; print(platform.python_version())')"
ok "python $PY_VERSION"

NEED_GO=1
if [ -n "${MONET_TUI_BIN:-}" ] && [ -x "${MONET_TUI_BIN}" ]; then
	NEED_GO=0
	ok "using prebuilt TUI at $MONET_TUI_BIN"
fi
if [ "$NEED_GO" = 1 ]; then
	have go || die "Go 1.24+ is required to build the TUI. https://go.dev/dl/ (or set MONET_TUI_BIN to a prebuilt binary)."
	GO_VERSION="$(go env GOVERSION 2>/dev/null || true)"
	[ -n "$GO_VERSION" ] || GO_VERSION="$(go version | awk '{print $3}')"
	GO_VERSION="${GO_VERSION#go}"
	GO_MAJOR="${GO_VERSION%%.*}"
	GO_REST="${GO_VERSION#*.}"
	GO_MINOR="${GO_REST%%.*}"
	GO_MINOR="${GO_MINOR%%[!0-9]*}"
	if [ "${GO_MAJOR:-0}" -lt 1 ] || { [ "${GO_MAJOR:-0}" -eq 1 ] && [ "${GO_MINOR:-0}" -lt 24 ]; }; then
		die "Go 1.24+ is required to build Monet Code (found $GO_VERSION). Update Go or set MONET_TUI_BIN."
	fi
	ok "go $GO_VERSION"
fi

# --------------------------------------------------------------------------- #
# fetch sources
# --------------------------------------------------------------------------- #

if [ -n "$SOURCE" ]; then
	[ -d "$SOURCE" ] || die "source directory not found: $SOURCE"
	[ -f "$SOURCE/go.mod" ] || die "$SOURCE does not look like a Monet Code checkout"
	step "installing from $SOURCE"
else
	step "fetching $REPO ($REF)"
fi

if [ -e "$INSTALL_DIR" ] && [ ! -d "$INSTALL_DIR" ]; then
	die "$INSTALL_DIR exists and is not a directory"
fi

if [ -d "$INSTALL_DIR" ] && [ -n "$(ls -A "$INSTALL_DIR" 2>/dev/null || true)" ]; then
	if [ -f "$INSTALL_DIR/$MARKER" ]; then
		:
	elif [ "$FORCE" = 1 ]; then
		rm -rf "$INSTALL_DIR"
	else
		die "$INSTALL_DIR is not empty and is not a Monet Code install. Choose another --dir or pass --force."
	fi
fi

mkdir -p "$INSTALL_DIR"

if [ -n "$SOURCE" ]; then
	( cd "$SOURCE" && tar \
		--exclude='./.git' \
		--exclude='./.venv' \
		--exclude='./bin' \
		--exclude='./monet_code.egg-info' \
		--exclude='*.egg-info' \
		--exclude='__pycache__' \
		-cf - . ) | ( cd "$INSTALL_DIR" && tar -xf - ) || die "could not copy the checkout"
else
	if [ -f "$INSTALL_DIR/$MARKER" ] && [ ! -d "$INSTALL_DIR/.git" ]; then
		# A previous --source install has no .git; replace it with a fresh clone.
		rm -rf "$INSTALL_DIR"
		mkdir -p "$INSTALL_DIR"
	fi
	if [ -d "$INSTALL_DIR/.git" ]; then
		step "updating the existing checkout"
		git -C "$INSTALL_DIR" remote set-url origin "https://github.com/$REPO.git" >/dev/null 2>&1 || true
		git -C "$INSTALL_DIR" fetch --quiet --depth 1 origin "$REF" || die "could not fetch $REF from $REPO"
		git -C "$INSTALL_DIR" reset --quiet --hard FETCH_HEAD || die "could not check out $REF"
	else
		git clone --quiet --depth 1 --branch "$REF" "https://github.com/$REPO.git" "$INSTALL_DIR" ||
			die "could not clone https://github.com/$REPO.git ($REF)"
	fi
fi

printf 'repo=%s\nref=%s\ninstalled=%s\n' "$REPO" "$REF" "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" >"$INSTALL_DIR/$MARKER"
ok "sources ready in $INSTALL_DIR"

# --------------------------------------------------------------------------- #
# build the TUI
# --------------------------------------------------------------------------- #

if [ "$NEED_GO" = 1 ]; then
	step "building the terminal UI"
	( cd "$INSTALL_DIR" && go build -trimpath -ldflags "-s -w" -o bin/monet-tui ./cmd/monet-tui ) ||
		die "the Go build failed. Run 'go build ./cmd/monet-tui' in $INSTALL_DIR for details."
	ok "built $INSTALL_DIR/bin/monet-tui"
else
	mkdir -p "$INSTALL_DIR/bin"
	ln -sfn "$MONET_TUI_BIN" "$INSTALL_DIR/bin/monet-tui"
	ok "linked the prebuilt TUI"
fi

# --------------------------------------------------------------------------- #
# Python environment
# --------------------------------------------------------------------------- #

step "setting up the Python CLI"
if ! "$PYTHON" -m venv "$INSTALL_DIR/.venv" >/dev/null 2>&1; then
	rm -rf "$INSTALL_DIR/.venv"
	die "could not create a virtualenv. On Debian/Ubuntu install python3-venv; on macOS install Xcode command line tools."
fi
VPY="$INSTALL_DIR/.venv/bin/python"
"$VPY" -m pip install --quiet --upgrade pip >/dev/null 2>&1 || true
"$VPY" -m pip install --quiet "$INSTALL_DIR" || die "pip could not install Monet Code (check your network)."
ok "installed the monet command"

# --------------------------------------------------------------------------- #
# shims
# --------------------------------------------------------------------------- #

step "writing launchers"
mkdir -p "$BIN_DIR"
cat >"$BIN_DIR/monet" <<EOF
#!/usr/bin/env bash
# monet-code installer
export MONET_TUI_BIN="\${MONET_TUI_BIN:-$INSTALL_DIR/bin/monet-tui}"
export MONET_REPO_DIR="$INSTALL_DIR"
exec "$INSTALL_DIR/.venv/bin/monet" "\$@"
EOF
chmod 0755 "$BIN_DIR/monet"
ln -sfn "$INSTALL_DIR/bin/monet-tui" "$BIN_DIR/monet-tui"
ok "$BIN_DIR/monet"

# --------------------------------------------------------------------------- #
# PATH
# --------------------------------------------------------------------------- #

case ":${PATH:-}:" in
*":$BIN_DIR:"*) ON_PATH=1 ;;
*) ON_PATH=0 ;;
esac

add_path_block() {
	local rc="$1"
	local line="$2"
	if [ -f "$rc" ] && grep -q "# >>> monet code >>>" "$rc"; then
		ok "$rc already has a Monet Code PATH block"
		return 0
	fi
	{
		printf '\n# >>> monet code >>>\n'
		printf '%s\n' "$line"
		printf '# <<< monet code <<<\n'
	} >>"$rc"
	ok "added $BIN_DIR to PATH in $rc"
}

if [ "$ON_PATH" = 0 ] && [ "$MODIFY_PATH" = 1 ]; then
	SHELL_NAME="$(basename "${SHELL:-sh}")"
	case "$SHELL_NAME" in
	zsh)
		add_path_block "${ZDOTDIR:-$HOME}/.zshrc" "export PATH=\"$BIN_DIR:\$PATH\""
		;;
	bash)
		RC="$HOME/.bashrc"
		[ "$(uname -s)" = Darwin ] && RC="$HOME/.bash_profile"
		add_path_block "$RC" "export PATH=\"$BIN_DIR:\$PATH\""
		;;
	fish)
		mkdir -p "$HOME/.config/fish"
		add_path_block "$HOME/.config/fish/config.fish" "fish_add_path \"$BIN_DIR\""
		;;
	*)
		warn "could not update PATH for $SHELL_NAME; add $BIN_DIR manually"
		;;
	esac
elif [ "$ON_PATH" = 0 ]; then
	warn "$BIN_DIR is not on your PATH"
fi

# --------------------------------------------------------------------------- #
# verify
# --------------------------------------------------------------------------- #

step "checking the install"
CLI_VERSION="$(unset MONET_TUI_BIN MONET_REPO_DIR; "$INSTALL_DIR/.venv/bin/monet" --version 2>/dev/null | tail -1 | sed 's/^[[:space:]]*//')"
[ -n "$CLI_VERSION" ] || die "the CLI did not start; run $INSTALL_DIR/.venv/bin/monet --version"
ok "monet $CLI_VERSION"

TUI_VERSION="$("$INSTALL_DIR/bin/monet-tui" --version 2>/dev/null || true)"
[ -n "$TUI_VERSION" ] || die "the TUI did not start; run $INSTALL_DIR/bin/monet-tui --version"
ok "$TUI_VERSION"

if have ollama; then
	if curl -fsS --max-time 3 "http://127.0.0.1:11434/api/version" >/dev/null 2>&1; then
		ok "ollama is running"
	else
		warn "ollama is installed but not reachable; run 'ollama serve' before chatting"
	fi
else
	warn "ollama was not found; install it from https://ollama.com to use the agent"
fi

# --------------------------------------------------------------------------- #
# done
# --------------------------------------------------------------------------- #

say ""
say "  ${BOLD}${TEAL}◆ Monet Code is installed${RESET}"
say ""
say "  ${DIM}install${RESET}  $INSTALL_DIR"
say "  ${DIM}launcher${RESET} $BIN_DIR/monet"
say ""
say "  ${BOLD}Try it${RESET}"
say "    ${FROST}monet${RESET}                    open the terminal UI"
say "    ${FROST}monet ask \"explain this repo\"${RESET}"
say "    ${FROST}monet models recommend${RESET}    browse the curated catalog"
say "    ${FROST}monet doctor${RESET}              check Ollama, Go and config"
say ""
if [ "$ON_PATH" = 0 ]; then
	say "  ${YELLOW}Add it to your PATH for this session:${RESET}"
	say "    export PATH=\"$BIN_DIR:\$PATH\""
	say "  ${DIM}or re-run with --modify-path to make it permanent.${RESET}"
	say ""
fi
say "  ${DIM}Uninstall:${RESET}"
say "    curl -fsSL https://raw.githubusercontent.com/$REPO/$REF/install.sh | bash -s -- --uninstall"
say ""
say "  ${DIM}Palette${RESET}  ${BLUE}#88C0D0${RESET} frost blue   ${TEAL}#8FBCBB${RESET} glacier teal   ${BLUE}#81A1C1${RESET} arctic blue"
say ""
