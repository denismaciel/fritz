#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "tree-sitter>=0.25.0",
#   "tree-sitter-rust>=0.24.0",
# ]
# ///

from __future__ import annotations

import argparse
import ast
import json
import re
from dataclasses import dataclass
from pathlib import Path

from tree_sitter import Language, Parser
from tree_sitter_rust import language as language_rust


ROOT = Path("/tmp/openai-codex")
DEFAULT_RS = ROOT / "codex-rs/core/src/compact.rs"
DEFAULT_OUT = Path("/home/denis/dotfiles/fritz/internal/prompt/reference/codex")
PROMPT_CONSTS = {
    "SUMMARIZATION_PROMPT": "compact_prompt.md",
    "SUMMARY_PREFIX": "summary_prefix.md",
}


@dataclass(frozen=True)
class ExtractedPrompt:
    const_name: str
    text: str
    source_file: str
    source_kind: str


def build_rust_parser() -> Parser:
    parser = Parser()
    parser.language = Language(language_rust())
    return parser


def node_text(source: bytes, node) -> str:
    return source[node.start_byte : node.end_byte].decode("utf-8")


def decode_rust_string_literal(literal: str) -> str:
    if literal.startswith("r"):
        match = re.fullmatch(r'r(#+)?"(.*)"\1', literal, flags=re.DOTALL)
        if match:
            return match.group(2)
        raise ValueError(f"unsupported raw string literal: {literal}")
    return ast.literal_eval(literal)


def extract_const_value_node(rs_path: Path, const_name: str):
    source = rs_path.read_bytes()
    tree = build_rust_parser().parse(source)
    stack = [tree.root_node]
    while stack:
        node = stack.pop()
        if node.type == "const_item":
            name_node = next((child for child in node.children if child.type == "identifier"), None)
            value_node = next(
                (
                    child
                    for child in node.children
                    if child.type in {"string_literal", "raw_string_literal", "macro_invocation"}
                ),
                None,
            )
            if name_node is None or value_node is None:
                stack.extend(reversed(node.children))
                continue
            if node_text(source, name_node) == const_name:
                return source, value_node
        stack.extend(reversed(node.children))
    raise ValueError(f"{const_name} not found in {rs_path}")


def decode_rust_const_expr(rs_path: Path, const_name: str) -> ExtractedPrompt:
    source, value_node = extract_const_value_node(rs_path, const_name)
    value_text = node_text(source, value_node)
    if value_node.type in {"string_literal", "raw_string_literal"}:
        return ExtractedPrompt(
            const_name=const_name,
            text=decode_rust_string_literal(value_text),
            source_file=str(rs_path),
            source_kind="inline_string",
        )
    if value_node.type == "macro_invocation":
        match = re.fullmatch(r'include_str!\((.+)\)', value_text, flags=re.DOTALL)
        if not match:
            raise ValueError(f"unsupported macro for {const_name}: {value_text}")
        include_path = decode_rust_string_literal(match.group(1).strip())
        resolved = (rs_path.parent / include_path).resolve()
        return ExtractedPrompt(
            const_name=const_name,
            text=resolved.read_text(encoding="utf-8"),
            source_file=str(resolved),
            source_kind="include_str",
        )
    raise ValueError(f"unsupported const expr for {const_name}: {value_node.type}")


def extract_prompts(rs_path: Path) -> dict[str, ExtractedPrompt]:
    return {name: decode_rust_const_expr(rs_path, name) for name in PROMPT_CONSTS}


def write_outputs(out_dir: Path, prompts: dict[str, ExtractedPrompt], rs_path: Path) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    for const_name, file_name in PROMPT_CONSTS.items():
        text = prompts[const_name].text
        if not text.endswith("\n"):
            text += "\n"
        (out_dir / file_name).write_text(text, encoding="utf-8")

    metadata = {
        "root": str(ROOT),
        "source_rs": str(rs_path),
        "prompts": {
            const_name: {
                "output_file": file_name,
                "source_file": prompts[const_name].source_file,
                "source_kind": prompts[const_name].source_kind,
            }
            for const_name, file_name in PROMPT_CONSTS.items()
        },
    }
    (out_dir / "sources.json").write_text(json.dumps(metadata, indent=2) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract OpenAI Codex compaction prompts.")
    parser.add_argument("--rs", type=Path, default=DEFAULT_RS)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    prompts = extract_prompts(args.rs)
    write_outputs(args.out, prompts, args.rs)
    print(f"wrote {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
