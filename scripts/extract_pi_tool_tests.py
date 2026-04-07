#!/usr/bin/env -S uv run
# /// script
# dependencies = ["tree-sitter", "tree-sitter-typescript"]
# ///

from __future__ import annotations

import argparse
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator

from tree_sitter import Language, Node, Parser
from tree_sitter_typescript import language_typescript


TEST_FN_NAMES = {"it", "test"}
SUITE_FN_NAMES = {"describe"}
TOOL_KEYWORDS = {
    "read",
    "write",
    "edit",
    "bash",
    "grep",
    "find",
    "ls",
}


@dataclass
class ExtractedTest:
    file: str
    line: int
    titles: list[str]
    imports: list[str]
    reasons: list[str]
    code: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Extract tool-related tests from Pi using the TypeScript AST."
    )
    parser.add_argument(
        "--pi-root",
        default="/tmp/pi-mono",
        help="Path to the cloned pi monorepo. Default: /tmp/pi-mono",
    )
    parser.add_argument(
        "--test-root",
        default="packages/coding-agent/test",
        help="Path relative to --pi-root to scan for tests.",
    )
    parser.add_argument(
        "--output",
        default="-",
        help="Output path. Use - for stdout.",
    )
    parser.add_argument(
        "--pretty",
        action="store_true",
        help="Pretty-print JSON output.",
    )
    return parser.parse_args()


def build_parser() -> Parser:
    parser = Parser(Language(language_typescript()))
    return parser


def main() -> int:
    args = parse_args()
    pi_root = Path(args.pi_root).resolve()
    test_root = (pi_root / args.test_root).resolve()
    parser = build_parser()

    results: list[dict[str, object]] = []
    for path in sorted(test_root.rglob("*.ts")):
        results.extend(extract_from_file(parser, pi_root, path))

    text = json.dumps(results, indent=2 if args.pretty else None)
    if args.output == "-":
        print(text)
    else:
        Path(args.output).write_text(text + ("\n" if not text.endswith("\n") else ""), encoding="utf-8")
    return 0


def extract_from_file(parser: Parser, pi_root: Path, path: Path) -> list[dict[str, object]]:
    source = path.read_bytes()
    tree = parser.parse(source)
    imports = collect_imports(tree.root_node, source)
    import_tool_related = any(
        any(
            segment in value.lower()
            for segment in (
                "/tools/read",
                "/tools/write",
                "/tools/edit",
                "/tools/bash",
                "/tools/grep",
                "/tools/find",
                "/tools/ls",
            )
        )
        for value in imports
    )

    tests: list[dict[str, object]] = []
    for test_node, titles in iter_test_cases(tree.root_node, source, []):
        reasons = classify_test(test_node, titles, imports, import_tool_related, source)
        if not reasons:
            continue

        test = ExtractedTest(
            file=str(path.relative_to(pi_root)),
            line=test_node.start_point.row + 1,
            titles=titles,
            imports=imports,
            reasons=reasons,
            code=node_text(test_node, source).strip(),
        )
        tests.append(
            {
                "file": test.file,
                "line": test.line,
                "titles": test.titles,
                "title": " > ".join(test.titles),
                "imports": test.imports,
                "reasons": test.reasons,
                "code": test.code,
            }
        )
    return tests


def collect_imports(root: Node, source: bytes) -> list[str]:
    imports: list[str] = []
    for child in root.children:
        if child.type != "import_statement":
            continue
        for grandchild in child.children:
            if grandchild.type == "string":
                imports.append(strip_quotes(node_text(grandchild, source)))
    return imports


def iter_test_cases(node: Node, source: bytes, suite_titles: list[str]) -> Iterator[tuple[Node, list[str]]]:
    if node.type == "call_expression":
        fn_name = get_call_name(node.child_by_field_name("function"), source)
        title = get_call_title(node, source)
        callback = get_call_callback(node)
        if fn_name in SUITE_FN_NAMES and title and callback is not None:
            next_titles = suite_titles + [title]
            yield from iter_test_cases(callback, source, next_titles)
            return
        if fn_name in TEST_FN_NAMES and title:
            yield node, suite_titles + [title]
            return

    for child in node.children:
        yield from iter_test_cases(child, source, suite_titles)


def classify_test(
    test_node: Node,
    titles: list[str],
    imports: list[str],
    import_tool_related: bool,
    source: bytes,
) -> list[str]:
    reasons: list[str] = []
    title_blob = " ".join(titles).lower()
    code_blob = node_text(test_node, source).lower()
    title_match = title_has_tool_keyword(title_blob)
    body_match = any(keyword in code_blob for keyword in build_code_keywords(imports))

    if import_tool_related and (title_match or body_match):
        reasons.append("imports_tool_module")

    if title_match:
        reasons.append("title_matches_tool_keyword")

    if body_match:
        reasons.append("body_mentions_tool_symbol")

    return sorted(set(reasons))


def build_code_keywords(imports: list[str]) -> set[str]:
    keywords = {
        '"read"',
        '"write"',
        '"edit"',
        '"bash"',
        '"grep"',
        '"find"',
        '"ls"',
        "'read'",
        "'write'",
        "'edit'",
        "'bash'",
        "'grep'",
        "'find'",
        "'ls'",
    }
    keywords.update(
        {
            "createreadtool",
            "createwritetool",
            "createedittool",
            "createbashtool",
            "creategreptool",
            "createfindtool",
            "createlstool",
            "readtool",
            "writetool",
            "edittool",
            "bashtool",
            "greptool",
            "findtool",
            "lstool",
            "/tools/read",
            "/tools/write",
            "/tools/edit",
            "/tools/bash",
            "/tools/grep",
            "/tools/find",
            "/tools/ls",
        }
    )
    for value in imports:
        lowered = value.lower()
        if any(
            segment in lowered
            for segment in (
                "/tools/read",
                "/tools/write",
                "/tools/edit",
                "/tools/bash",
                "/tools/grep",
                "/tools/find",
                "/tools/ls",
            )
        ):
            keywords.add(value.lower())
    return keywords


def title_has_tool_keyword(title_blob: str) -> bool:
    patterns = [
        r"\bread\b",
        r"\bwrite\b",
        r"\bedit\b",
        r"\bbash\b",
        r"\bgrep\b",
        r"\bfind\b",
        r"\bls\b",
        r"\btool execution\b",
        r"\bfile mutation\b",
    ]
    return any(re.search(pattern, title_blob) for pattern in patterns)


def get_call_name(function_node: Node | None, source: bytes) -> str | None:
    if function_node is None:
        return None
    if function_node.type == "identifier":
        return node_text(function_node, source)
    if function_node.type == "member_expression":
        property_node = function_node.child_by_field_name("property")
        if property_node is not None:
            return node_text(property_node, source)
    return None


def get_call_title(call_node: Node, source: bytes) -> str | None:
    arguments = call_node.child_by_field_name("arguments")
    if arguments is None:
        return None
    for child in arguments.named_children:
        if child.type in {"string", "template_string"}:
            return clean_string_literal(node_text(child, source))
    return None


def get_call_callback(call_node: Node) -> Node | None:
    arguments = call_node.child_by_field_name("arguments")
    if arguments is None:
        return None
    for child in arguments.named_children:
        if child.type in {"arrow_function", "function_expression"}:
            body = child.child_by_field_name("body")
            return body if body is not None else child
    return None


def node_text(node: Node, source: bytes) -> str:
    return source[node.start_byte : node.end_byte].decode("utf-8")


def clean_string_literal(text: str) -> str:
    text = text.strip()
    if len(text) >= 2 and text[0] == text[-1] and text[0] in {"'", '"', "`"}:
        return text[1:-1]
    return text


def strip_quotes(text: str) -> str:
    return clean_string_literal(text)


if __name__ == "__main__":
    raise SystemExit(main())
