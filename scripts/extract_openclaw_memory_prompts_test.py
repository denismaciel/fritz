#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "commonmark>=0.9.1",
#   "tree-sitter>=0.25.0",
#   "tree-sitter-typescript>=0.23.2",
# ]
# ///

from __future__ import annotations

import tempfile
from pathlib import Path

from extract_openclaw_memory_prompts import (
    extract_exported_const_string,
    extract_markdown_sections,
    write_outputs,
)


def main() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        ts_path = root / "heartbeat.ts"
        ts_path.write_text(
            'export const HEARTBEAT_PROMPT = "Read HEARTBEAT.md. If nothing matters, reply HEARTBEAT_OK.";\n'
        )
        md_path = root / "AGENTS.md"
        md_path.write_text(
            "# Memory\n\nUse MEMORY.md.\n\n## Memory child\n\nNested detail.\n\n# Heartbeats\n\nRead HEARTBEAT.md.\n\n# Other\n\nIgnore.\n"
        )
        out_dir = root / "out"

        prompt = extract_exported_const_string(ts_path, "HEARTBEAT_PROMPT")
        assert prompt == "Read HEARTBEAT.md. If nothing matters, reply HEARTBEAT_OK."

        sections = extract_markdown_sections(md_path)
        assert [section.heading for section in sections] == ["Memory", "Heartbeats"]

        write_outputs(out_dir, prompt, sections)
        assert (out_dir / "heartbeat_prompt.txt").read_text().strip() == prompt
        assert "Use MEMORY.md." in (out_dir / "memory_sections.md").read_text()
        assert "Read HEARTBEAT.md." in (out_dir / "heartbeat_sections.md").read_text()

    print("ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
