from __future__ import annotations

import argparse
import json
import re
from pathlib import Path

DEFAULT_TS = Path("/tmp/pi-mono/packages/coding-agent/src/core/system-prompt.ts")
DEFAULT_OUT = Path("/home/denis/dotfiles/fritz/internal/prompt/reference/pi")


def extract_default_prompt_template(ts_path: Path) -> str:
    text = ts_path.read_text(encoding="utf-8")
    match = re.search(r"let prompt = `(?P<body>.*?)`;\n", text, re.DOTALL)
    if not match:
        raise ValueError(f"could not find default prompt template in {ts_path}")
    return match.group("body").strip() + "\n"


def write_outputs(out_dir: Path, template: str, ts_path: Path) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    (out_dir / "default_system_prompt_template.md").write_text(template, encoding="utf-8")
    (out_dir / "sources.json").write_text(
        json.dumps(
            {
                "template_source": str(ts_path),
                "output_file": "default_system_prompt_template.md",
            },
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )


def main() -> None:
    parser = argparse.ArgumentParser(description="Extract Pi coding-agent default system prompt template.")
    parser.add_argument("--ts", type=Path, default=DEFAULT_TS)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    args = parser.parse_args()

    template = extract_default_prompt_template(args.ts)
    write_outputs(args.out, template, args.ts)


if __name__ == "__main__":
    main()
