#!/usr/bin/env -S uv run
# /// script
# dependencies = ["tree-sitter", "tree-sitter-typescript", "tree-sitter-go"]
# ///

from __future__ import annotations

import importlib.util
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("validate_pi_tool_test_parity.py")
SPEC = importlib.util.spec_from_file_location("validate_pi_tool_test_parity", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class NormalizeTitleTests(unittest.TestCase):
    def test_normalize_title_aligns_pi_and_go_styles(self) -> None:
        self.assertEqual(
            MODULE.normalize_title("should preserve UTF-8 BOM after edit"),
            MODULE.normalize_title("TestPreserveUTF8BOMAfterEdit"),
        )


class ExtractTsTests(unittest.TestCase):
    def test_extracts_describe_and_it_titles(self) -> None:
        parser = MODULE.build_ts_parser()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = root / "tools.test.ts"
            path.write_text(
                textwrap.dedent(
                    """
                    describe("read tool", () => {
                      it("should read file contents", () => {});
                      test("should handle offsets", () => {});
                    });
                    """
                ),
                encoding="utf-8",
            )
            cases = MODULE.extract_ts_tests(path, root, parser)

        self.assertEqual([case.title for case in cases], ["should read file contents", "should handle offsets"])
        self.assertEqual(cases[0].parents, ("read tool",))


class ExtractGoTests(unittest.TestCase):
    def test_extracts_top_level_and_subtests(self) -> None:
        parser = MODULE.build_go_parser()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = root / "tool_test.go"
            path.write_text(
                textwrap.dedent(
                    """
                    package tool

                    import "testing"

                    func TestWriteCreatesFile(t *testing.T) {}

                    func TestReadTool(t *testing.T) {
                        t.Run("should read file contents", func(t *testing.T) {})
                    }
                    """
                ),
                encoding="utf-8",
            )
            cases = MODULE.extract_go_tests(path, root, parser)

        self.assertEqual([case.title for case in cases], ["Write Creates File", "should read file contents"])
        self.assertEqual(cases[1].parents, ("Read Tool",))


class PackFilteringTests(unittest.TestCase):
    def test_filters_pi_cases_to_pack(self) -> None:
        pi_cases = [
            MODULE.TestCase(source="pi", file="a.ts", line=1, title="should read file contents"),
            MODULE.TestCase(source="pi", file="a.ts", line=2, title="should write file contents"),
        ]
        filtered = MODULE.filter_pi_cases_to_pack(pi_cases, ["should write file contents"])
        self.assertEqual([case.title for case in filtered], ["should write file contents"])


if __name__ == "__main__":
    unittest.main()
