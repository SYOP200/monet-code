#!/usr/bin/env bash
#
# Monet Code installer
#   curl -fsSL https://raw.githubusercontent.com/SYOP200/monet-code/main/install.sh | bash
#
# Downloads a prebuilt TUI binary and a self-contained Python CLI for macOS and
# Linux. A Go toolchain is not required; Python 3.9+ is only needed for the
# `monet` subcommands (the TUI itself is a static binary).
#
# Options (pass after `bash -s --`):
#   --dir PATH        install location    (default: ~/.local/share/monet-code)
#   --bin-dir PATH    shim location       (default: ~/.local/bin)
#   --ref REF         release tag, or a branch to build from source (default: latest release)
#   --modify-path     append BIN_DIR to your shell rc file
#   --force           replace an existing install
#   --uninstall       remove Monet Code
#   --source PATH     install from a local checkout (development; needs Go)
#   -h, --help        show this help
#
# Environment:
#   MONET_HOME, MONET_BIN_DIR, MONET_REF, MONET_REPO,
#   MONET_RELEASE_BASE, MONET_INSTALL_SOURCE, MONET_TUI_BIN, NO_COLOR
#
set -euo pipefail

REPO="${MONET_REPO:-SYOP200/monet-code}"
REF="${MONET_REF:-latest}"
RELEASE_BASE="${MONET_RELEASE_BASE:-https://github.com/$REPO/releases/download}"
DEFAULT_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/monet-code"
INSTALL_DIR="${MONET_HOME:-$DEFAULT_HOME}"
BIN_DIR="${MONET_BIN_DIR:-$HOME/.local/bin}"
SOURCE="${MONET_INSTALL_SOURCE:-}"

MODIFY_PATH=0
FORCE=0
UNINSTALL=0
MARKER=".monet-code-install"
HAVE_PYTHON=0
PYTHON=""
MODE=""
TAG=""
TMP=""

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
  --bin-dir PATH   where to write the monet launcher (default: ~/.local/bin)
  --ref REF        release tag to install, or a branch to build from source
                   (default: the latest published release)
  --modify-path    append the bin directory to your shell rc file
  --force          replace an existing install
  --uninstall      remove Monet Code from this machine
  --source PATH    install from a local checkout instead of a release
  -h, --help       show this help

Environment:
  MONET_HOME, MONET_BIN_DIR, MONET_REF, MONET_REPO,
  MONET_RELEASE_BASE, MONET_INSTALL_SOURCE, MONET_TUI_BIN, NO_COLOR

A normal install downloads prebuilt binaries, so Go and Git are not required.
Python 3.9+ powers the `monet` subcommands; without it the launcher still opens
the TUI. Building a branch with --ref or installing with --source needs Go.

After installing:
  monet              open the TUI
  monet ask "hi"     one-shot prompt
  monet doctor       check Python, Ollama, the TUI and the config
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
	if [ -L "$BIN_DIR/monet-tui" ] || [ -f "$BIN_DIR/monet-tui" ]; then
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
# platform and dependencies
# --------------------------------------------------------------------------- #

say ""
say "  ${BOLD}◆ MONET CODE${RESET} ${DIM}a local-first coding agent for the terminal${RESET}"
say ""

step "checking dependencies"

case "$(uname -s)" in
Darwin) OS=darwin ;;
Linux) OS=linux ;;
*) die "unsupported platform: $(uname -s). Monet Code targets macOS and Linux." ;;
esac

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
arm64 | aarch64) ARCH=arm64 ;;
*) die "unsupported architecture: $(uname -m). Prebuilt binaries cover amd64 and arm64." ;;
esac
ok "$OS $ARCH"

have curl || die "curl is required. Install curl and try again."
have tar || die "tar is required. Install tar and try again."

if have python3; then
	PYTHON=python3
elif have python; then
	PYTHON=python
fi
if [ -n "$PYTHON" ] && "$PYTHON" - <<'PY' >/dev/null 2>&1
import sys
raise SystemExit(0 if sys.version_info >= (3, 9) else 1)
PY
then
	HAVE_PYTHON=1
	PY_VERSION="$("$PYTHON" -c 'import platform; print(platform.python_version())')"
	ok "python $PY_VERSION"
else
	PYTHON=""
	warn "Python 3.9+ not found; installing the TUI only (monet will open the TUI, but monet ask/models need Python)"
fi

# --------------------------------------------------------------------------- #
# choose release, source checkout or branch build
# --------------------------------------------------------------------------- #

resolve_latest_tag() {
	local url
	url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
		"https://github.com/$REPO/releases/latest" 2>/dev/null || true)"
	case "$url" in
	*/tag/*) printf '%s' "${url##*/tag/}" ;;
	*) printf '' ;;
	esac
}

if [ -n "$SOURCE" ]; then
	MODE=source
elif [ "$REF" = "latest" ] || [ "$REF" = "stable" ]; then
	TAG="$(resolve_latest_tag)"
	if [ -z "$TAG" ]; then
		die "no published release found for $REPO. Install a branch with --ref main (requires Go) or pass --source."
	fi
	MODE=release
else
	case "$REF" in
	v[0-9]*) TAG="$REF" ;;
	[0-9]*) TAG="v$REF" ;;
	*) TAG="" ;;
	esac
	if [ -n "$TAG" ]; then
		MODE=release
	else
		MODE=branch
	fi
fi

# --------------------------------------------------------------------------- #
# install directory
# --------------------------------------------------------------------------- #

if [ -e "$INSTALL_DIR" ] && [ ! -d "$INSTALL_DIR" ]; then
	die "$INSTALL_DIR exists and is not a directory"
fi

if [ -d "$INSTALL_DIR" ] && [ -n "$(ls -A "$INSTALL_DIR" 2>/dev/null || true)" ]; then
	if [ -f "$INSTALL_DIR/$MARKER" ]; then
		rm -rf "$INSTALL_DIR"
	elif [ "$FORCE" = 1 ]; then
		rm -rf "$INSTALL_DIR"
	else
		die "$INSTALL_DIR is not empty and is not a Monet Code install. Choose another --dir or pass --force."
	fi
fi
mkdir -p "$INSTALL_DIR"

# --------------------------------------------------------------------------- #
# helpers
# --------------------------------------------------------------------------- #

fetch() {
	local url="$1" out="$2"
	curl -fsSL --show-error --retry 3 --retry-delay 1 --connect-timeout 15 -o "$out" "$url" ||
		die "could not download $url"
}

sha256_of() {
	if have sha256sum; then
		sha256sum "$1" | awk '{print $1}'
	elif have shasum; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		return 1
	fi
}

verify_asset() {
	local file="$1" name="$2" expected actual
	expected="$(awk -v n="$name" '$2 == n || $2 == "*" n { print $1; exit }' "$TMP/checksums.txt")"
	[ -n "$expected" ] || die "no checksum published for $name"
	if ! actual="$(sha256_of "$file")"; then
		warn "no sha256 tool found; skipping checksum verification"
		return 0
	fi
	[ "$expected" = "$actual" ] || die "checksum mismatch for $name (download may be corrupt)"
	ok "verified $name"
}

need_go() {
	have go || die "Go 1.24+ is required to build from source. Use the default prebuilt install, or install Go: https://go.dev/dl/"
	GO_VERSION="$(go env GOVERSION 2>/dev/null || true)"
	[ -n "$GO_VERSION" ] || GO_VERSION="$(go version | awk '{print $3}')"
	GO_VERSION="${GO_VERSION#go}"
	GO_MAJOR="${GO_VERSION%%.*}"
	GO_REST="${GO_VERSION#*.}"
	GO_MINOR="${GO_REST%%.*}"
	GO_MINOR="${GO_MINOR%%[!0-9]*}"
	if [ "${GO_MAJOR:-0}" -lt 1 ] || { [ "${GO_MAJOR:-0}" -eq 1 ] && [ "${GO_MINOR:-0}" -lt 24 ]; }; then
		die "Go 1.24+ is required to build Monet Code (found $GO_VERSION)."
	fi
	ok "go $GO_VERSION"
}

write_marker() {
	{
		printf 'repo=%s\n' "$REPO"
		printf 'mode=%s\n' "$MODE"
		if [ -n "$TAG" ]; then printf 'tag=%s\n' "$TAG"; fi
		if [ "$MODE" = source ]; then
			printf 'source=%s\n' "$SOURCE"
		elif [ -n "$REF" ]; then
			printf 'ref=%s\n' "$REF"
		fi
		printf 'installed=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
	} >"$INSTALL_DIR/$MARKER"
}

write_launcher_release() {
	mkdir -p "$BIN_DIR"
	cat >"$BIN_DIR/monet" <<EOF
#!/usr/bin/env bash
# monet-code installer
export MONET_HOME="$INSTALL_DIR"
export MONET_TUI_BIN="\${MONET_TUI_BIN:-$INSTALL_DIR/bin/monet-tui}"
if [ -d "$INSTALL_DIR/cli/monet_cli" ]; then
	if command -v python3 >/dev/null 2>&1; then
		MONET_PY=python3
	elif command -v python >/dev/null 2>&1; then
		MONET_PY=python
	fi
	if [ -n "\${MONET_PY:-}" ]; then
		export PYTHONPATH="$INSTALL_DIR/cli\${PYTHONPATH:+:\$PYTHONPATH}"
		exec "\$MONET_PY" -m monet_cli "\$@"
	fi
fi
exec "\$MONET_TUI_BIN" "\$@"
EOF
	chmod 0755 "$BIN_DIR/monet"
}

write_launcher_dev() {
	mkdir -p "$BIN_DIR"
	cat >"$BIN_DIR/monet" <<EOF
#!/usr/bin/env bash
# monet-code installer
export MONET_HOME="$INSTALL_DIR"
export MONET_TUI_BIN="\${MONET_TUI_BIN:-$INSTALL_DIR/bin/monet-tui}"
export MONET_REPO_DIR="$INSTALL_DIR"
exec "$INSTALL_DIR/.venv/bin/monet" "\$@"
EOF
	chmod 0755 "$BIN_DIR/monet"
}

# --------------------------------------------------------------------------- #
# release install
# --------------------------------------------------------------------------- #

if [ "$MODE" = release ]; then
	step "downloading Monet Code $TAG"
	TMP="$(mktemp -d "${TMPDIR:-/tmp}/monet-install.XXXXXX")"
	trap 'rm -rf "$TMP"' EXIT
	mkdir -p "$INSTALL_DIR/bin"

	fetch "$RELEASE_BASE/$TAG/checksums.txt" "$TMP/checksums.txt"

	if [ -n "${MONET_TUI_BIN:-}" ] && [ -x "${MONET_TUI_BIN}" ]; then
		mkdir -p "$INSTALL_DIR/bin"
		ln -sfn "$MONET_TUI_BIN" "$INSTALL_DIR/bin/monet-tui"
		ok "using prebuilt TUI at $MONET_TUI_BIN"
	else
		TUI_ASSET="monet-tui-$OS-$ARCH"
		fetch "$RELEASE_BASE/$TAG/$TUI_ASSET" "$TMP/$TUI_ASSET"
		verify_asset "$TMP/$TUI_ASSET" "$TUI_ASSET"
		install -m 0755 "$TMP/$TUI_ASSET" "$INSTALL_DIR/bin/monet-tui"
		ok "installed the TUI for $OS $ARCH"
	fi

	if [ "$HAVE_PYTHON" = 1 ]; then
		fetch "$RELEASE_BASE/$TAG/monet-cli.tar.gz" "$TMP/monet-cli.tar.gz"
		verify_asset "$TMP/monet-cli.tar.gz" "monet-cli.tar.gz"
		mkdir -p "$INSTALL_DIR/cli"
		tar -xzf "$TMP/monet-cli.tar.gz" -C "$INSTALL_DIR/cli" ||
			die "could not unpack the CLI bundle"
		[ -f "$INSTALL_DIR/cli/monet_cli/__main__.py" ] ||
			die "the CLI bundle is incomplete"
		ok "installed the self-contained CLI"
	else
		warn "skipping the CLI bundle (Python 3.9+ not found)"
	fi

	write_marker
	write_launcher_release
fi

# --------------------------------------------------------------------------- #
# source and branch installs (development)
# --------------------------------------------------------------------------- #

if [ "$MODE" = source ] || [ "$MODE" = branch ]; then
	if [ "$MODE" = source ]; then
		[ -d "$SOURCE" ] || die "source directory not found: $SOURCE"
		[ -f "$SOURCE/go.mod" ] || die "$SOURCE does not look like a Monet Code checkout"
		step "installing from $SOURCE"
	else
		step "fetching $REPO ($REF)"
		TMP="$(mktemp -d "${TMPDIR:-/tmp}/monet-install.XXXXXX")"
		trap 'rm -rf "$TMP"' EXIT
		fetch "https://codeload.github.com/$REPO/tar.gz/$REF" "$TMP/source.tar.gz"
		mkdir -p "$INSTALL_DIR"
		tar -xzf "$TMP/source.tar.gz" -C "$INSTALL_DIR" --strip-components=1 ||
			die "could not unpack the source archive"
	fi

	mkdir -p "$INSTALL_DIR"

	if [ "$MODE" = source ]; then
		( cd "$SOURCE" && tar \
			--exclude='./.git' \
			--exclude='./.venv' \
			--exclude='./bin' \
			--exclude='./dist' \
			--exclude='./build' \
			--exclude='./monet_code.egg-info' \
			--exclude='*.egg-info' \
			--exclude='__pycache__' \
			-cf - . ) | ( cd "$INSTALL_DIR" && tar -xf - ) || die "could not copy the checkout"
	fi

	if [ -n "${MONET_TUI_BIN:-}" ] && [ -x "${MONET_TUI_BIN}" ]; then
		mkdir -p "$INSTALL_DIR/bin"
		ln -sfn "$MONET_TUI_BIN" "$INSTALL_DIR/bin/monet-tui"
		ok "using prebuilt TUI at $MONET_TUI_BIN"
	else
		need_go
		step "building the terminal UI"
		( cd "$INSTALL_DIR" && go build -trimpath -ldflags "-s -w" -o bin/monet-tui ./cmd/monet-tui ) ||
			die "the Go build failed. Run 'go build ./cmd/monet-tui' in $INSTALL_DIR for details."
		ok "built $INSTALL_DIR/bin/monet-tui"
	fi

	[ "$HAVE_PYTHON" = 1 ] || die "Python 3.9+ is required for a source install."
	step "setting up the Python CLI"
	if ! "$PYTHON" -m venv "$INSTALL_DIR/.venv" >/dev/null 2>&1; then
		rm -rf "$INSTALL_DIR/.venv"
		die "could not create a virtualenv. On Arch install python-pip; on Debian/Ubuntu install python3-venv; on macOS install the Xcode command line tools."
	fi
	VPY="$INSTALL_DIR/.venv/bin/python"
	"$VPY" -m pip install --quiet --upgrade pip >/dev/null 2>&1 || true
	"$VPY" -m pip install --quiet "$INSTALL_DIR" || die "pip could not install Monet Code (check your network)."
	ok "installed the monet command"

	write_marker
	write_launcher_dev
fi

# --------------------------------------------------------------------------- #
# PATH
# --------------------------------------------------------------------------- #

mkdir -p "$BIN_DIR"
ln -sfn "$INSTALL_DIR/bin/monet-tui" "$BIN_DIR/monet-tui"

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
TUI_VERSION="$("$INSTALL_DIR/bin/monet-tui" --version 2>/dev/null || true)"
[ -n "$TUI_VERSION" ] || die "the TUI did not start; run $INSTALL_DIR/bin/monet-tui --version"
ok "$TUI_VERSION"

if [ "$HAVE_PYTHON" = 1 ] && [ -d "$INSTALL_DIR/cli/monet_cli" ]; then
	CLI_VERSION="$(MONET_HOME="$INSTALL_DIR" MONET_TUI_BIN="$INSTALL_DIR/bin/monet-tui" \
		PYTHONPATH="$INSTALL_DIR/cli" "$PYTHON" -m monet_cli --version 2>/dev/null |
		awk 'NF { v = $NF } END { print v }')"
	[ -n "$CLI_VERSION" ] || die "the CLI did not start; run $BIN_DIR/monet --version"
	ok "monet $CLI_VERSION"
elif [ -x "$INSTALL_DIR/.venv/bin/monet" ]; then
	CLI_VERSION="$(unset MONET_TUI_BIN MONET_REPO_DIR; "$INSTALL_DIR/.venv/bin/monet" --version 2>/dev/null | tail -1 | sed 's/^[[:space:]]*//')"
	[ -n "$CLI_VERSION" ] || die "the CLI did not start; run $INSTALL_DIR/.venv/bin/monet --version"
	ok "monet $CLI_VERSION"
fi

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
if [ -n "$TAG" ]; then
	say "  ${DIM}release${RESET}  $TAG"
fi
say ""
say "  ${BOLD}Try it${RESET}"
say "    ${FROST}monet${RESET}                    open the terminal UI"
say "    ${FROST}monet ask \"explain this repo\"${RESET}"
say "    ${FROST}monet models recommend${RESET}    browse the curated catalog"
say "    ${FROST}monet doctor${RESET}              check Python, Ollama, the TUI and config"
say ""
if [ "$ON_PATH" = 0 ]; then
	say "  ${YELLOW}Add it to your PATH for this session:${RESET}"
	say "    export PATH=\"$BIN_DIR:\$PATH\""
	say "  ${DIM}or re-run with --modify-path to make it permanent.${RESET}"
	say ""
fi
say "  ${DIM}Uninstall:${RESET}"
say "    curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | bash -s -- --uninstall"
say ""
say "  ${DIM}Palette${RESET}  ${BLUE}#88C0D0${RESET} frost blue   ${TEAL}#8FBCBB${RESET} glacier teal   ${BLUE}#81A1C1${RESET} arctic blue"
say ""
