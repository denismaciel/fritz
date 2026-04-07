#!/usr/bin/env -S uv run
# /// script
# dependencies = ["tree-sitter", "tree-sitter-rust", "tree-sitter-go"]
# ///

from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("validate_codex_compaction_test_parity.py")
SPEC = importlib.util.spec_from_file_location("validate_codex_compaction_test_parity", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class NormalizeTitleTests(unittest.TestCase):
    def test_normalize_title_aligns_rust_and_go_styles(self) -> None:
        self.assertEqual(
            MODULE.normalize_title("auto_compaction_remote_emits_started_and_completed_items"),
            MODULE.normalize_title("TestAutoCompactionRemoteEmitsStartedAndCompletedItems"),
        )


class ExtractGoTests(unittest.TestCase):
    def test_extracts_top_level_and_subtests(self) -> None:
        parser = MODULE.build_go_parser()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = root / "compaction_test.go"
            path.write_text(
                textwrap.dedent(
                    """
                    package session

                    import "testing"

                    func TestAutoCompactionRemoteEmitsStartedAndCompletedItems(t *testing.T) {}

                    func TestCompaction(t *testing.T) {
                        t.Run("resume after compaction", func(t *testing.T) {})
                    }
                    """
                ),
                encoding="utf-8",
            )
            cases = MODULE.extract_go_tests(path, root, parser)

        self.assertEqual(
            [case.title for case in cases],
            ["Auto Compaction Remote Emits Started And Completed Items", "resume after compaction"],
        )


class LoadCodexCasesTests(unittest.TestCase):
    def test_loads_json_inventory(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "compaction-tests.json"
            path.write_text(
                json.dumps([{"file": "a.rs", "line": 12, "title": "auto_compaction_runs"}]),
                encoding="utf-8",
            )
            cases = MODULE.load_codex_cases(path)
        self.assertEqual(cases[0].title, "auto_compaction_runs")


if __name__ == "__main__":
    unittest.main()
