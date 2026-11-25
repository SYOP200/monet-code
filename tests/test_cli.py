"""Tests for Monet Code's Python CLI helpers."""

from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path

from monet_cli import catalog, config, organize


class OrganizeTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self.root = Path(self._tmp.name)

    def tearDown(self) -> None:
        self._tmp.cleanup()

    def test_classify(self) -> None:
        self.assertEqual(organize.classify("notes.md")[0], "Documents")
        self.assertEqual(organize.classify("photo.JPG")[0], "Images")
        self.assertEqual(organize.classify("main.go")[0], "Code")
        self.assertEqual(organize.classify("LICENSE")[0], "Other")

    def test_analyze_skips_hidden_and_directories(self) -> None:
        (self.root / "notes.md").write_text("hello")
        (self.root / "photo.png").write_text("png")
        (self.root / ".secret").write_text("hidden")
        (self.root / "src").mkdir()
        report = organize.analyze(self.root)
        self.assertEqual(report.total_files, 2)
        self.assertIn("Documents", report.groups)
        self.assertIn("Images", report.groups)
        self.assertNotIn("Other", report.groups)

    def test_plan_is_dry_run_and_apply_moves(self) -> None:
        (self.root / "a.md").write_text("doc")
        (self.root / "b.py").write_text("print(1)")
        moves = organize.plan(self.root)
        self.assertEqual(len(moves), 2)
        self.assertFalse((self.root / "Documents").exists())
        moved, errors = organize.apply(moves)
        self.assertEqual((moved, errors), (2, []))
        self.assertTrue((self.root / "Documents" / "a.md").is_file())
        self.assertTrue((self.root / "Code" / "b.py").is_file())

    def test_plan_avoids_name_collisions(self) -> None:
        (self.root / "notes.md").write_text("loose")
        (self.root / "Documents").mkdir()
        (self.root / "Documents" / "notes.md").write_text("filed")
        moves = organize.plan(self.root)
        self.assertEqual(len(moves), 1)
        self.assertEqual(moves[0].rel_to, "Documents/notes (1).md")


class ConfigTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self._old = os.environ.get("MONET_CONFIG_DIR")
        os.environ["MONET_CONFIG_DIR"] = self._tmp.name

    def tearDown(self) -> None:
        if self._old is None:
            os.environ.pop("MONET_CONFIG_DIR", None)
        else:
            os.environ["MONET_CONFIG_DIR"] = self._old
        self._tmp.cleanup()

    def test_defaults_and_roundtrip(self) -> None:
        cfg = config.load()
        self.assertEqual(cfg["editor_tab_size"], 4)
        self.assertTrue(cfg["show_thinking"])
        config.set_model(cfg, "qwen3.5:4b")
        path = config.save(cfg)
        self.assertEqual(path, Path(self._tmp.name) / "config.json")
        reloaded = config.load()
        self.assertEqual(reloaded["model"], "qwen3.5:4b")
        self.assertEqual(reloaded["recent_models"][0], "qwen3.5:4b")

    def test_parse_value(self) -> None:
        self.assertEqual(config.parse_value("editor_tab_size", "2"), 2)
        self.assertTrue(config.parse_value("show_thinking", "yes"))
        self.assertFalse(config.parse_value("show_thinking", "off"))
        self.assertEqual(config.parse_value("recent_models", "a, b"), ["a", "b"])

    def test_malformed_file_falls_back(self) -> None:
        (Path(self._tmp.name) / "config.json").write_text("{broken")
        cfg = config.load()
        self.assertEqual(cfg["editor_tab_size"], 4)
        self.assertTrue((Path(self._tmp.name) / "config.json.bak").exists())

    def test_ollama_url_normalization(self) -> None:
        cfg = dict(config.DEFAULTS)
        cfg["ollama_url"] = "127.0.0.1:11434/"
        self.assertEqual(config.ollama_url(cfg), "http://127.0.0.1:11434")


class CatalogTests(unittest.TestCase):
    def test_catalog_matches_go_source(self) -> None:
        path = catalog.catalog_path()
        self.assertIsNotNone(path)
        entries = catalog.entries()
        self.assertGreaterEqual(len(entries), 10)
        for entry in entries:
            self.assertTrue(entry.get("name"))
            self.assertTrue(entry.get("description"))

    def test_find_is_tag_insensitive(self) -> None:
        self.assertIsNotNone(catalog.find("qwen3-coder:30b"))
        self.assertIsNone(catalog.find("does-not-exist"))

    def test_synced_copy_matches_source(self) -> None:
        source = Path(catalog.REPO_ROOT) / "internal" / "catalog" / "models.json"
        synced = Path(catalog.REPO_ROOT) / "monet_cli" / "models.json"
        if source.is_file() and synced.is_file():
            self.assertEqual(
                json.loads(source.read_text(encoding="utf-8")),
                json.loads(synced.read_text(encoding="utf-8")),
                "monet_cli/models.json is stale — run `make sync-catalog`",
            )


if __name__ == "__main__":
    unittest.main()
