#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "tree-sitter>=0.25.0",
#   "tree-sitter-rust>=0.24.0",
# ]
# ///

from __future__ import annotations

import tempfile
from pathlib import Path

from extract_codex_compaction_prompts import extract_prompts, write_outputs


def main() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        templates = root / "templates" / "compact"
        templates.mkdir(parents=True)
        (templates / "prompt.md").write_text("Summarize the conversation.\n", encoding="utf-8")
        (templates / "summary_prefix.md").write_text("Prior model summary prefix.\n", encoding="utf-8")
        rs_path = root / "compact.rs"
        rs_path.write_text(
            (
                'pub const SUMMARIZATION_PROMPT: &str = include_str!("templates/compact/prompt.md");\n'
                'pub const SUMMARY_PREFIX: &str = include_str!("templates/compact/summary_prefix.md");\n'
            ),
            encoding="utf-8",
        )

        prompts = extract_prompts(rs_path)
        assert prompts["SUMMARIZATION_PROMPT"].text == "Summarize the conversation.\n"
        assert prompts["SUMMARY_PREFIX"].text == "Prior model summary prefix.\n"

        out_dir = root / "out"
        write_outputs(out_dir, prompts, rs_path)
        assert (out_dir / "compact_prompt.md").read_text(encoding="utf-8") == "Summarize the conversation.\n"
        assert (out_dir / "summary_prefix.md").read_text(encoding="utf-8") == "Prior model summary prefix.\n"
        assert "SUMMARIZATION_PROMPT" in (out_dir / "sources.json").read_text(encoding="utf-8")

    print("ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
