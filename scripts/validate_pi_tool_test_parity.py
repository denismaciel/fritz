#!/usr/bin/env -S uv run
# /// script
# dependencies = ["tree-sitter", "tree-sitter-typescript", "tree-sitter-go"]
# ///

from __future__ import annotations

import argparse
import difflib
import re
import sys
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Iterator

from tree_sitter import Language, Node, Parser
from tree_sitter_go import language as language_go
from tree_sitter_typescript import language_typescript


DEFAULT_PI_FILES = (
    "packages/coding-agent/test/path-utils.test.ts",
    "packages/coding-agent/test/tools.test.ts",
    "packages/coding-agent/test/edit-tool-legacy-input.test.ts",
    "packages/coding-agent/test/file-mutation-queue.test.ts",
    "packages/coding-agent/test/bash-close-hang-windows.test.ts",
    "packages/coding-agent/test/rpc.test.ts",
    "packages/coding-agent/test/block-images.test.ts",
)


@dataclass(frozen=True)
class TestCase:
    source: str
    file: str
    line: int
    title: str
    parents: tuple[str, ...] = ()

    @property
    def full_title(self) -> str:
        if not self.parents:
            return self.title
        return " > ".join([*self.parents, self.title])

    @property
    def normalized(self) -> str:
        return normalize_title(self.title)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Compare Pi tool parity test names vs local Go tests."
    )
    parser.add_argument(
        "--repo-root",
        default=".",
        help="Local repo root. Default: cwd",
    )
    parser.add_argument(
        "--pi-root",
        default="/tmp/pi-mono",
        help="Pi monorepo root. Default: /tmp/pi-mono",
    )
    parser.add_argument(
        "--pack-dir",
        default="docs/tasks/pi-tool-parity",
        help="Pack dir used to scope parity test names.",
    )
    parser.add_argument(
        "--show-matched",
        action="store_true",
        help="Print matched tests too.",
    )
    return parser.parse_args()


def build_ts_parser() -> Parser:
    return Parser(Language(language_typescript()))


def build_go_parser() -> Parser:
    return Parser(Language(language_go()))


def normalize_title(title: str) -> str:
    text = unicodedata.normalize("NFKC", title)
    text = camel_to_words(text)
    text = text.casefold()
    text = text.replace("utf-8", "utf 8")
    text = re.sub(r"[_/\\\-]+", " ", text)
    text = re.sub(r"([a-z])([0-9])", r"\1 \2", text)
    text = re.sub(r"([0-9])([a-z])", r"\1 \2", text)
    text = re.sub(r"^[^a-z0-9]+", "", text)
    text = re.sub(r"\b(?:test|should)\b", " ", text)
    text = re.sub(r"[^a-z0-9]+", " ", text)
    text = re.sub(r"\s+", " ", text).strip()
    return text


def camel_to_words(text: str) -> str:
    text = re.sub(r"(?<=[a-z0-9])(?=[A-Z])", " ", text)
    text = re.sub(r"(?<=[A-Z])(?=[A-Z][a-z])", " ", text)
    return text


def parse_pack_checklists(pack_dir: Path) -> list[str]:
    names: list[str] = []
    for path in sorted(pack_dir.glob("PTP-*.md")):
        for line in path.read_text(encoding="utf-8").splitlines():
            match = re.match(r"^- \[[ xX]\] (.+?)\s*$", line)
            if match:
                names.append(match.group(1))
    return names


def extract_ts_tests(path: Path, root: Path, parser: Parser) -> list[TestCase]:
    source = path.read_bytes()
    tree = parser.parse(source)
    cases: list[TestCase] = []
    for node, parents, title in iter_ts_test_cases(tree.root_node, source, ()):
        cases.append(
            TestCase(
                source="pi",
                file=str(path.relative_to(root)),
                line=node.start_point.row + 1,
                title=title,
                parents=parents,
            )
        )
    return cases


def iter_ts_test_cases(
    node: Node, source: bytes, parents: tuple[str, ...]
) -> Iterator[tuple[Node, tuple[str, ...], str]]:
    if node.type == "call_expression":
        func_name = get_ts_call_name(node.child_by_field_name("function"), source)
        title = get_ts_call_title(node, source)
        callback = get_ts_call_callback(node)
        if func_name == "describe" and title and callback is not None:
            yield from iter_ts_test_cases(callback, source, (*parents, title))
            return
        if func_name in {"it", "test"} and title:
            yield node, parents, title
            return
    for child in node.children:
        yield from iter_ts_test_cases(child, source, parents)


def get_ts_call_name(node: Node | None, source: bytes) -> str | None:
    if node is None:
        return None
    if node.type == "identifier":
        return node_text(node, source)
    if node.type == "member_expression":
        property_node = node.child_by_field_name("property")
        if property_node is not None:
            return node_text(property_node, source)
    return None


def get_ts_call_title(node: Node, source: bytes) -> str | None:
    args = node.child_by_field_name("arguments")
    if args is None:
        return None
    for child in args.children:
        if child.type in {"string", "template_string"}:
            return strip_quotes(node_text(child, source))
    return None


def get_ts_call_callback(node: Node) -> Node | None:
    args = node.child_by_field_name("arguments")
    if args is None:
        return None
    for child in args.children:
        if child.type in {"arrow_function", "function"}:
            body = child.child_by_field_name("body")
            return body or child
    return None


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


def iter_go_subtests(node: Node, source: bytes) -> Iterator[tuple[Node, str]]:
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


def node_text(node: Node | None, source: bytes) -> str:
    if node is None:
        return ""
    return source[node.start_byte : node.end_byte].decode("utf-8")


def strip_quotes(value: str) -> str:
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'", "`"}:
        return value[1:-1]
    return value


def filter_pi_cases_to_pack(pi_cases: Iterable[TestCase], target_names: Iterable[str]) -> list[TestCase]:
    wanted = {normalize_title(name) for name in target_names}
    return [case for case in pi_cases if case.normalized in wanted]


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


def collect_local_cases(repo_root: Path, parser: Parser) -> list[TestCase]:
    cases: list[TestCase] = []
    for rel in ("internal/tool", "internal/app", "internal/chat"):
        root = repo_root / rel
        if not root.exists():
            continue
        for path in sorted(root.rglob("*_test.go")):
            cases.extend(extract_go_tests(path, repo_root, parser))
    return cases


def collect_pi_cases(pi_root: Path, parser: Parser) -> list[TestCase]:
    cases: list[TestCase] = []
    for rel in DEFAULT_PI_FILES:
        path = pi_root / rel
        if not path.exists():
            continue
        cases.extend(extract_ts_tests(path, pi_root, parser))
    return cases


def print_case_group(header: str, cases: list[TestCase], stream: object = sys.stdout) -> None:
    print(header, file=stream)
    for case in cases:
        print(f"  - {case.title} [{case.file}:{case.line}]", file=stream)


def main() -> int:
    args = parse_args()
    repo_root = Path(args.repo_root).resolve()
    pi_root = Path(args.pi_root).resolve()
    pack_dir = (repo_root / args.pack_dir).resolve()

    target_names = parse_pack_checklists(pack_dir)
    ts_parser = build_ts_parser()
    go_parser = build_go_parser()

    pi_cases = collect_pi_cases(pi_root, ts_parser)
    scoped_pi_cases = filter_pi_cases_to_pack(pi_cases, target_names)
    local_cases = collect_local_cases(repo_root, go_parser)

    target_index = build_index(
        TestCase(source="pack", file="pack", line=0, title=name) for name in target_names
    )
    pi_index = build_index(scoped_pi_cases)
    local_index = build_index(local_cases)

    pack_missing_in_pi = [
        next(iter(cases))
        for key, cases in target_index.items()
        if key not in pi_index
    ]

    missing: list[tuple[TestCase, list[str]]] = []
    for key, cases in target_index.items():
        if key in local_index:
            continue
        pack_case = next(iter(cases))
        missing.append((pack_case, closest_titles(pack_case.title, local_cases)))

    extra = [
        case
        for case in local_cases
        if case.normalized not in target_index
    ]

    matched_count = sum(1 for key in target_index if key in local_index)

    print("Pi tool parity test validator", flush=True)
    print(f"  target tests: {len(target_index)}", flush=True)
    print(f"  pi scoped tests: {len(pi_index)}", flush=True)
    print(f"  local tests: {len(local_cases)}", flush=True)
    print(f"  matched: {matched_count}", flush=True)

    if pack_missing_in_pi:
        print("\nPACK/PI MISMATCH", file=sys.stderr)
        for case in pack_missing_in_pi:
            print(f"  - {case.title}", file=sys.stderr)

    if args.show_matched:
        matched = [
            local_index[key][0]
            for key in sorted(target_index)
            if key in local_index
        ]
        print_case_group("\nMatched", matched)

    if extra:
        print_case_group("\nExtra local tests (info)", extra)

    if missing:
        print("\n" + "!" * 72, file=sys.stderr)
        print("MISSING PI PARITY TESTS", file=sys.stderr)
        print("!" * 72, file=sys.stderr)
        for case, suggestions in missing:
            print(f"- {case.title}", file=sys.stderr)
            if suggestions:
                print(f"  closest local: {', '.join(suggestions)}", file=sys.stderr)
        return 1

    print("\nAll scoped Pi parity tests have a local test name match.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
