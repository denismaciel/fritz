from __future__ import annotations

import tempfile
from pathlib import Path

from extract_pi_system_prompt import extract_default_prompt_template, write_outputs


def test_extract_default_prompt_template() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        ts_path = Path(tmp) / "system-prompt.ts"
        ts_path.write_text(
            """
export function buildSystemPrompt(): string {
\tlet prompt = `You are an expert coding assistant.

Available tools:
${toolsList}

Guidelines:
${guidelines}`;
\treturn prompt;
}
""".strip()
            + "\n",
            encoding="utf-8",
        )

        template = extract_default_prompt_template(ts_path)

        assert "You are an expert coding assistant." in template
        assert "${toolsList}" in template
        assert "${guidelines}" in template


def test_write_outputs() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        out_dir = Path(tmp) / "out"
        ts_path = Path(tmp) / "system-prompt.ts"
        ts_path.write_text("let prompt = `hello`;\\n", encoding="utf-8")

        write_outputs(out_dir, "hello\n", ts_path)

        assert (out_dir / "default_system_prompt_template.md").read_text(encoding="utf-8") == "hello\n"
        assert "system-prompt.ts" in (out_dir / "sources.json").read_text(encoding="utf-8")
