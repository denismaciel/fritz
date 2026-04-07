#!/usr/bin/env -S uv run
# /// script
# dependencies = ["tree-sitter", "tree-sitter-rust", "tree-sitter-go"]
# ///

from __future__ import annotations

import argparse
import difflib
import json
import re
import sys
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

from tree_sitter import Language, Node, Parser
from tree_sitter_go import language as language_go
from tree_sitter_rust import language as language_rust


DEFAULT_CODEX_TESTS = "docs/reference/codex/compaction-tests.json"
DEFAULT_PACK_DIR = "docs/tasks/context-compaction-parity"
LOCAL_SCAN_ROOTS = ("internal/session", "internal/agent", "internal/chat", "internal/prompt")


@dataclass(frozen=True)
class TestCase:
    source: str
    file: str
    line: int
    title: str
    parents: tuple[str, ...] = ()

    @property
    def normalized(self) -> str:
        return normalize_title(self.title)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Compare extracted Codex compaction test names vs local Go tests."
    )
    parser.add_argument("--repo-root", default=".", help="Local repo root. Default: cwd")
    parser.add_argument(
        "--codex-tests",
        default=DEFAULT_CODEX_TESTS,
        help="Path to extracted Codex compaction test inventory JSON.",
    )
    parser.add_argument(
        "--pack-dir",
        default=DEFAULT_PACK_DIR,
        help="Pack dir used to scope adopted Codex compaction parity tests.",
    )
    parser.add_argument("--show-matched", action="store_true", help="Print matched tests too.")
    return parser.parse_args()


def build_rust_parser() -> Parser:
    return Parser(Language(language_rust()))


def build_go_parser() -> Parser:
    return Parser(Language(language_go()))


def camel_to_words(text: str) -> str:
    text = re.sub(r"(?<=[a-z0-9])(?=[A-Z])", " ", text)
    text = re.sub(r"(?<=[A-Z])(?=[A-Z][a-z])", " ", text)
    return text


def normalize_title(title: str) -> str:
    text = unicodedata.normalize("NFKC", title)
    text = camel_to_words(text)
    text = text.casefold()
    text = text.replace("utf-8", "utf 8")
    text = re.sub(r"[_/\\\-]+", " ", text)
    text = re.sub(r"([a-z])([0-9])", r"\1 \2", text)
    text = re.sub(r"([0-9])([a-z])", r"\1 \2", text)
    text = re.sub(r"^[^a-z0-9]+", "", text)
    text = re.sub(r"\b(?:test|should|async|fn)\b", " ", text)
    text = re.sub(r"[^a-z0-9]+", " ", text)
    return re.sub(r"\s+", " ", text).strip()


def node_text(node: Node | None, source: bytes) -> str:
    if node is None:
        return ""
    return source[node.start_byte : node.end_byte].decode("utf-8")


def strip_quotes(value: str) -> str:
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'", "`"}:
        return value[1:-1]
    return value


def extract_go_tests(path: Path, root: Path, parser: Parser) -> list[TestCase]:
    source = path.read_bytes()
    tree = parser.parse(source)
    cases: list[TestCase] = []
    for child in tree.root_node.children:
        if child.type != "function_declaration":
            continue
        name_node = child.child_by_field_name("name")
        if name_node is None:
            continue
        func_name = node_text(name_node, source)
        if not func_name.startswith("Test"):
            continue
        body = child.child_by_field_name("body")
        if body is None:
            continue
        parent_title = camel_to_words(func_name.removeprefix("Test")).strip()
        subtests = list(iter_go_subtests(body, source))
        if subtests:
            for node, title in subtests:
                cases.append(
                    TestCase(
                        source="local",
                        file=str(path.relative_to(root)),
                        line=node.start_point.row + 1,
                        title=title,
                        parents=(parent_title,),
                    )
                )
            continue
        cases.append(
            TestCase(
                source="local",
                file=str(path.relative_to(root)),
                line=child.start_point.row + 1,
                title=parent_title,
            )
        )
    return cases


def iter_go_subtests(node: Node, source: bytes):
    if node.type == "call_expression":
        fn_node = node.child_by_field_name("function")
        fn_text = node_text(fn_node, source) if fn_node is not None else ""
        if fn_text.endswith(".Run"):
            title = get_go_call_title(node, source)
            if title:
                yield node, title
    for child in node.children:
        yield from iter_go_subtests(child, source)


def get_go_call_title(node: Node, source: bytes) -> str | None:
    args = node.child_by_field_name("arguments")
    if args is None:
        return None
    for child in args.children:
        if child.type in {"interpreted_string_literal", "raw_string_literal"}:
            return strip_quotes(node_text(child, source))
    return None


def collect_local_cases(repo_root: Path, parser: Parser) -> list[TestCase]:
    cases: list[TestCase] = []
    for rel in LOCAL_SCAN_ROOTS:
        root = repo_root / rel
        if not root.exists():
            continue
        for path in sorted(root.rglob("*_test.go")):
            cases.extend(extract_go_tests(path, repo_root, parser))
    return cases


def parse_pack_checklists(pack_dir: Path) -> list[str]:
    names: list[str] = []
    for path in sorted(pack_dir.glob("*.md")):
        for line in path.read_text(encoding="utf-8").splitlines():
            match = re.match(r"^- \[[ xX]\] (.+?)\s*$", line)
            if match:
                names.append(match.group(1))
    return names


def load_codex_cases(path: Path) -> list[TestCase]:
    raw = json.loads(path.read_text(encoding="utf-8"))
    return [
        TestCase(
            source="codex",
            file=item["file"],
            line=item["line"],
            title=item["title"],
        )
        for item in raw
    ]


def filter_codex_cases_to_pack(cases: Iterable[TestCase], target_names: Iterable[str]) -> list[TestCase]:
    wanted = {normalize_title(name) for name in target_names}
    if not wanted:
        return list(cases)
    return [case for case in cases if case.normalized in wanted]


def build_index(cases: Iterable[TestCase]) -> dict[str, list[TestCase]]:
    index: dict[str, list[TestCase]] = {}
    for case in cases:
        index.setdefault(case.normalized, []).append(case)
    return index


def closest_titles(title: str, local_cases: list[TestCase], limit: int = 3) -> list[str]:
    normalized_to_title = {case.normalized: case.title for case in local_cases}
    matches = difflib.get_close_matches(
        normalize_title(title),
        list(normalized_to_title),
        n=limit,
        cutoff=0.45,
    )
    return [normalized_to_title[key] for key in matches]


def print_case_group(header: str, cases: list[TestCase], stream: object = sys.stdout) -> None:
    print(header, file=stream)
    for case in cases:
        print(f"  - {case.title} [{case.file}:{case.line}]", file=stream)


def main() -> int:
    args = parse_args()
    repo_root = Path(args.repo_root).resolve()
    pack_dir = (repo_root / args.pack_dir).resolve()
    codex_tests_path = (repo_root / args.codex_tests).resolve()
    go_parser = build_go_parser()

    target_names = parse_pack_checklists(pack_dir)
    codex_cases = filter_codex_cases_to_pack(load_codex_cases(codex_tests_path), target_names)
    local_cases = collect_local_cases(repo_root, go_parser)

    codex_index = build_index(codex_cases)
    local_index = build_index(local_cases)

    missing: list[tuple[TestCase, list[str]]] = []
    for key, cases in codex_index.items():
        if key in local_index:
            continue
        codex_case = next(iter(cases))
        missing.append((codex_case, closest_titles(codex_case.title, local_cases)))

    extra = [case for case in local_cases if case.normalized not in codex_index]
    matched_count = sum(1 for key in codex_index if key in local_index)

    print("Codex compaction parity validator", flush=True)
    print(f"  codex tests: {len(codex_index)}", flush=True)
    print(f"  local tests: {len(local_cases)}", flush=True)
    print(f"  matched: {matched_count}", flush=True)

    if args.show_matched:
        matched = [local_index[key][0] for key in sorted(codex_index) if key in local_index]
        print_case_group("\nMatched", matched)

    if extra:
        print_case_group("\nExtra local tests (info)", extra)

    if missing:
        print("\n" + "!" * 72, file=sys.stderr)
        print("MISSING CODEX COMPACTION PARITY TESTS", file=sys.stderr)
        print("!" * 72, file=sys.stderr)
        for case, suggestions in missing:
            print(f"- {case.title}", file=sys.stderr)
            if suggestions:
                print(f"  closest local: {', '.join(suggestions)}", file=sys.stderr)
        return 1

    print("\nAll extracted Codex compaction tests have a local test name match.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
