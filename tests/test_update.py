import hashlib
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from monet_cli import update


def write_checksums(directory: Path, names) -> None:
    lines = []
    for name in names:
        digest = hashlib.sha256((directory / name).read_bytes()).hexdigest()
        lines.append(f"{digest}  {name}")
    (directory / "checksums.txt").write_text("\n".join(lines) + "\n")


def make_release(directory: Path, tui_name: str, tui: bytes, cli_main: bytes) -> None:
    directory.mkdir(parents=True, exist_ok=True)
    (directory / tui_name).write_bytes(tui)

    tree = directory.parent / "tree"
    (tree / "monet_cli").mkdir(parents=True, exist_ok=True)
    (tree / "monet_cli" / "__main__.py").write_bytes(cli_main)
    with tarfile.open(directory / "monet-cli.tar.gz", "w:gz") as bundle:
        bundle.add(tree / "monet_cli", arcname="monet_cli")

    write_checksums(directory, [tui_name, "monet-cli.tar.gz"])


def make_install(root: Path, tag: str = "v0.2.0") -> None:
    (root / "bin").mkdir(parents=True)
    (root / "bin" / "monet-tui").write_bytes(b"old tui")
    (root / "cli" / "monet_cli").mkdir(parents=True)
    (root / "cli" / "monet_cli" / "__main__.py").write_bytes(b"old cli")
    (root / update.MARKER).write_text(
        f"repo=acme/monet\nmode=release\ntag={tag}\nref={tag}\n", encoding="utf-8"
    )


class VersionTests(unittest.TestCase):
    def test_normalize_tag(self):
        self.assertEqual(update.normalize_tag("0.2.0"), "v0.2.0")
        self.assertEqual(update.normalize_tag("v0.2.0"), "v0.2.0")
        self.assertEqual(update.normalize_tag(""), "")

    def test_version_order(self):
        self.assertTrue(update.is_newer("v0.2.1", "v0.2.0"))
        self.assertTrue(update.is_newer("v0.10.0", "v0.9.9"))
        self.assertFalse(update.is_newer("v0.2.0", "v0.2.0"))
        self.assertFalse(update.is_newer("v0.2.0", "v0.2.1"))

    def test_asset_names(self):
        self.assertEqual(
            update.asset_names("linux", "x86_64"), ("monet-tui-linux-amd64", "linux amd64")
        )
        self.assertEqual(
            update.asset_names("darwin", "arm64"), ("monet-tui-darwin-arm64", "darwin arm64")
        )
        with self.assertRaises(update.UpdateError):
            update.asset_names("plan9", "x86_64")

    def test_parse_marker_and_checksums(self):
        marker = update.parse_marker("repo=acme/monet\nmode=release\ntag=v1.2.3\n")
        self.assertEqual(marker["repo"], "acme/monet")
        self.assertEqual(marker["tag"], "v1.2.3")
        sums = update.parse_checksums("abc  one\nDEF *two\n")
        self.assertEqual(sums, {"one": "abc", "two": "def"})


class UpdateTests(unittest.TestCase):
    def test_local_release_update(self):
        with tempfile.TemporaryDirectory() as temp:
            temp = Path(temp)
            root = temp / "install"
            make_install(root)
            release = temp / "release" / "v0.2.1"
            make_release(
                release,
                "monet-tui-linux-amd64",
                b"new tui",
                b"new cli",
            )

            with mock.patch.object(
                update, "asset_names", return_value=("monet-tui-linux-amd64", "linux amd64")
            ):
                result = update.run_update(
                    root=root,
                    repo="acme/monet",
                    tag="v0.2.1",
                    base=release.parent.as_uri(),
                )

            self.assertTrue(result.updated)
            self.assertEqual(result.current, "v0.2.0")
            self.assertEqual(result.latest, "v0.2.1")
            self.assertEqual((root / "bin" / "monet-tui").read_bytes(), b"new tui")
            self.assertEqual((root / "cli" / "monet_cli" / "__main__.py").read_bytes(), b"new cli")
            marker = update.parse_marker((root / update.MARKER).read_text())
            self.assertEqual(marker["tag"], "v0.2.1")
            self.assertFalse((root / "cli.old").exists())
            self.assertFalse((root / "cli.new").exists())

    def test_check_does_not_change_files(self):
        with tempfile.TemporaryDirectory() as temp:
            temp = Path(temp)
            root = temp / "install"
            make_install(root)
            result = update.run_update(
                root=root, repo="acme/monet", tag="v0.9.0", check=True
            )
            self.assertFalse(result.updated)
            self.assertEqual(result.latest, "v0.9.0")
            self.assertEqual((root / "bin" / "monet-tui").read_bytes(), b"old tui")

    def test_up_to_date_is_a_noop(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "install"
            make_install(root, tag="v0.2.0")
            result = update.run_update(root=root, repo="acme/monet", tag="0.2.0")
            self.assertFalse(result.updated)
            self.assertEqual(result.current, "v0.2.0")

    def test_refuses_custom_symlink(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "install"
            make_install(root)
            target = Path(temp) / "custom-tui"
            target.write_bytes(b"custom")
            binary = root / "bin" / "monet-tui"
            binary.unlink()
            binary.symlink_to(target)
            with self.assertRaises(update.UpdateError):
                update.run_update(root=root, repo="acme/monet", tag="v0.2.1")

    def test_source_install_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "install"
            make_install(root)
            (root / update.MARKER).write_text("repo=acme/monet\nmode=source\nref=main\n")
            with self.assertRaises(update.UpdateError):
                update.run_update(root=root, repo="acme/monet", tag="v0.2.1")


if __name__ == "__main__":
    unittest.main()
