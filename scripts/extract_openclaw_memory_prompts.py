#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "commonmark>=0.9.1",
#   "tree-sitter>=0.25.0",
#   "tree-sitter-typescript>=0.23.2",
# ]
# ///

from __future__ import annotations

import argparse
import ast
import json
from dataclasses import dataclass
from pathlib import Path

import commonmark
from tree_sitter import Language, Parser
from tree_sitter_typescript import language_typescript


ROOT = Path("/home/denis/tmp/openclaw-EUwHTl")
DEFAULT_TS = ROOT / "src/auto-reply/heartbeat.ts"
DEFAULT_MD = [
    ROOT / "docs/reference/templates/AGENTS.md",
    ROOT / "docs/reference/AGENTS.default.md",
    ROOT / "docs/concepts/system-prompt.md",
]
DEFAULT_OUT = Path("/home/denis/dotfiles/fritz/internal/prompt/reference/openclaw")


@dataclass
class Section:
    source: str
    heading: str
    level: int
    body: str


def build_ts_parser() -> Parser:
    parser = Parser()
    parser.language = Language(language_typescript())
    return parser


def node_text(source: bytes, node) -> str:
    return source[node.start_byte : node.end_byte].decode("utf-8")


def decode_ts_string(source: bytes, node) -> str:
    kind = node.type
    if kind == "string":
        return ast.literal_eval(node_text(source, node))
    if kind == "template_string":
        parts: list[str] = []
        for child in node.children:
            if child.type == "string_fragment":
                parts.append(node_text(source, child))
        return "".join(parts)
    if kind == "parenthesized_expression":
        inner = next((child for child in node.children if child.type != "(" and child.type != ")"), None)
        if inner is None:
            raise ValueError("empty parenthesized expression")
        return decode_ts_string(source, inner)
    if kind == "binary_expression":
        left, right = None, None
        for child in node.children:
            if child.type == "+":
                continue
            if left is None:
                left = child
            else:
                right = child
                break
        if left is None or right is None:
            raise ValueError("invalid binary expression")
        return decode_ts_string(source, left) + decode_ts_string(source, right)
    raise ValueError(f"unsupported ts string node: {kind}")


def extract_exported_const_string(ts_path: Path, const_name: str) -> str:
    source = ts_path.read_bytes()
    tree = build_ts_parser().parse(source)
    stack = [tree.root_node]
    while stack:
        node = stack.pop()
        if node.type == "variable_declarator":
            name_node = node.child_by_field_name("name")
            value_node = node.child_by_field_name("value")
            if name_node is None or value_node is None:
                continue
            if node_text(source, name_node) == const_name:
                return decode_ts_string(source, value_node).strip()
        stack.extend(reversed(node.children))
    raise ValueError(f"{const_name} not found in {ts_path}")


def heading_text(node) -> str:
    parts: list[str] = []
    walker = node.walker()
    for child, entering in walker:
        if not entering:
            continue
        if child.t == "text" and child.literal:
            parts.append(child.literal)
        if child.t == "code" and child.literal:
            parts.append(child.literal)
    return "".join(parts).strip()


def extract_markdown_sections(md_path: Path) -> list[Section]:
    text = md_path.read_text()
    parser = commonmark.Parser()
    root = parser.parse(text)
    headings: list[tuple[int, int, str]] = []
    walker = root.walker()
    for node, entering in walker:
        if not entering or node.t != "heading":
            continue
        if not node.sourcepos:
            continue
        start_line = node.sourcepos[0][0]
        headings.append((start_line, node.level, heading_text(node)))

    lines = text.splitlines()
    sections: list[Section] = []
    covered_until = 0
    for index, (start_line, level, title) in enumerate(headings):
        title_folded = title.casefold()
        if "memory" not in title_folded and "heartbeat" not in title_folded:
            continue
        if start_line <= covered_until:
            continue
        end_line = len(lines)
        for next_start, next_level, _ in headings[index + 1 :]:
            if next_level <= level:
                end_line = next_start - 1
                break
        body = "\n".join(lines[start_line - 1 : end_line]).strip()
        sections.append(Section(source=str(md_path), heading=title, level=level, body=body))
        covered_until = end_line
    return sections


def render_sections(sections: list[Section]) -> str:
    blocks: list[str] = []
    for section in sections:
        blocks.append(f"<!-- source: {section.source} -->\n{section.body}")
    return "\n\n".join(blocks).strip() + "\n"


def write_outputs(out_dir: Path, heartbeat_prompt: str, sections: list[Section]) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    memory_sections = [section for section in sections if "memory" in section.heading.casefold()]
    heartbeat_sections = [section for section in sections if "heartbeat" in section.heading.casefold()]

    (out_dir / "heartbeat_prompt.txt").write_text(heartbeat_prompt + "\n")
    (out_dir / "memory_sections.md").write_text(render_sections(memory_sections))
    (out_dir / "heartbeat_sections.md").write_text(render_sections(heartbeat_sections))
    (out_dir / "sources.json").write_text(
        json.dumps(
            {
                "heartbeat_prompt_source": str(DEFAULT_TS),
                "markdown_sources": [str(path) for path in DEFAULT_MD],
                "memory_section_count": len(memory_sections),
                "heartbeat_section_count": len(heartbeat_sections),
            },
            indent=2,
        )
        + "\n"
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract OpenClaw memory/heartbeat prompt material.")
    parser.add_argument("--ts", type=Path, default=DEFAULT_TS)
    parser.add_argument("--md", type=Path, nargs="*", default=DEFAULT_MD)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    heartbeat_prompt = extract_exported_const_string(args.ts, "HEARTBEAT_PROMPT")
    sections: list[Section] = []
    for md_path in args.md:
        sections.extend(extract_markdown_sections(md_path))
    write_outputs(args.out, heartbeat_prompt, sections)
    print(f"wrote {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
