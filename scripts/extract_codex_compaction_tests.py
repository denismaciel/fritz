#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "tree-sitter>=0.25.0",
#   "tree-sitter-rust>=0.24.0",
# ]
# ///

from __future__ import annotations

import argparse
import json
from dataclasses import asdict
from dataclasses import dataclass
from pathlib import Path

from tree_sitter import Language, Node, Parser
from tree_sitter_rust import language as language_rust


ROOT = Path("/tmp/openai-codex")
DEFAULT_SCAN_ROOTS = (
    "codex-rs/core/src",
    "codex-rs/core/tests/suite",
    "codex-rs/app-server/tests/suite",
)
KEYWORDS = ("compact", "compaction")


@dataclass(frozen=True)
class ExtractedRustTest:
    file: str
    line: int
    name: str
    attrs: tuple[str, ...]


def build_rust_parser() -> Parser:
    parser = Parser()
    parser.language = Language(language_rust())
    return parser


def node_text(node: Node | None, source: bytes) -> str:
    if node is None:
        return ""
    return source[node.start_byte : node.end_byte].decode("utf-8")


def is_test_attr(attr_text: str) -> bool:
    return attr_text.startswith("#[test") or attr_text.startswith("#[tokio::test")


def extract_rust_tests(path: Path, root: Path, parser: Parser) -> list[ExtractedRustTest]:
    source = path.read_bytes()
    tree = parser.parse(source)
    cases: list[ExtractedRustTest] = []
    for fn_node, attrs in iter_test_functions(tree.root_node, source):
        name_node = next((child for child in fn_node.children if child.type == "identifier"), None)
        if name_node is None:
            continue
        cases.append(
            ExtractedRustTest(
                file=str(path.relative_to(root)),
                line=fn_node.start_point.row + 1,
                name=node_text(name_node, source),
                attrs=attrs,
            )
        )
    return cases


def iter_test_functions(node: Node, source: bytes):
    pending_attrs: list[str] = []
    for child in node.children:
        if child.type == "attribute_item":
            pending_attrs.append(node_text(child, source))
            continue
        if child.type == "function_item":
            attrs = tuple(pending_attrs)
            if any(is_test_attr(attr) for attr in attrs):
                yield child, attrs
            pending_attrs = []
            yield from iter_test_functions(child, source)
            continue
        pending_attrs = []
        yield from iter_test_functions(child, source)


def is_compaction_test(test: ExtractedRustTest) -> bool:
    lowered = test.name.casefold()
    return any(keyword in lowered for keyword in KEYWORDS)


def collect_compaction_tests(root: Path, parser: Parser) -> list[ExtractedRustTest]:
    cases: list[ExtractedRustTest] = []
    for rel in DEFAULT_SCAN_ROOTS:
        scan_root = root / rel
        if not scan_root.exists():
            continue
        for path in sorted(scan_root.rglob("*.rs")):
            for case in extract_rust_tests(path, root, parser):
                if is_compaction_test(case):
                    cases.append(case)
    return cases


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract OpenAI Codex compaction-related Rust test names.")
    parser.add_argument("--codex-root", type=Path, default=ROOT)
    parser.add_argument("--output", default="-", help="Output path. Use - for stdout.")
    parser.add_argument("--pretty", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    parser = build_rust_parser()
    cases = collect_compaction_tests(args.codex_root, parser)
    payload = [
        {
            **asdict(case),
            "title": case.name,
        }
        for case in cases
    ]
    text = json.dumps(payload, indent=2 if args.pretty else None)
    if args.output == "-":
        print(text)
    else:
        out_path = Path(args.output)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(text + ("\n" if not text.endswith("\n") else ""), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
