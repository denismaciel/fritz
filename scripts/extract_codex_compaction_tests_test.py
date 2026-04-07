#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "tree-sitter>=0.25.0",
#   "tree-sitter-rust>=0.24.0",
# ]
# ///

from __future__ import annotations

import tempfile
import textwrap
from pathlib import Path

from extract_codex_compaction_tests import build_rust_parser, collect_compaction_tests, extract_rust_tests


def main() -> int:
    parser = build_rust_parser()
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        path = root / "suite.rs"
        path.write_text(
            textwrap.dedent(
                """
                fn helper() {}

                #[test]
                fn build_compacted_history_truncates_overlong_user_messages() {}

                #[tokio::test(flavor = "multi_thread")]
                async fn auto_compaction_remote_emits_started_and_completed_items() {}

                #[test]
                fn unrelated_history_behavior() {}
                """
            ),
            encoding="utf-8",
        )

        extracted = extract_rust_tests(path, root, parser)
        assert [case.name for case in extracted] == [
            "build_compacted_history_truncates_overlong_user_messages",
            "auto_compaction_remote_emits_started_and_completed_items",
            "unrelated_history_behavior",
        ]

        scan_root = root / "codex-rs" / "core" / "tests" / "suite"
        scan_root.mkdir(parents=True)
        (scan_root / "compact.rs").write_text(path.read_text(encoding="utf-8"), encoding="utf-8")
        compact_cases = collect_compaction_tests(root, parser)
        assert [case.name for case in compact_cases] == [
            "build_compacted_history_truncates_overlong_user_messages",
            "auto_compaction_remote_emits_started_and_completed_items",
        ]

    print("ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
