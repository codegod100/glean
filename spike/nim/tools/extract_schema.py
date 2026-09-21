#!/usr/bin/env python3
"""Regenerate atproto/schema.nim from the Go schema.

The DDL lives in internal/db/db.go as three []string literals. Retyping 44
statements into Nim would drift -- a differing default or CHECK constraint
surfaces months later as a violation nobody can place -- so they are copied
mechanically instead.

    python3 tools/extract_schema.py
"""
import pathlib
import re
import sys

GO = pathlib.Path(__file__).resolve().parents[3] / "internal" / "db" / "db.go"
OUT = pathlib.Path(__file__).resolve().parents[1] / "atproto" / "schema.nim"

HEADER = '''## Glean's schema, in the three databases it uses.
##
## Extracted mechanically from internal/db/db.go rather than retyped: 44 DDL
## statements transcribed by hand would drift from the Go definition, and a
## column that differs by a default or a CHECK is the kind of thing that only
## shows up as a constraint violation months later.
##
## Regenerate with tools/extract_schema.py after changing the Go schema.

'''


def groups(src):
    """Locate each `var <name>Schema = []string{ ... }` block by line."""
    lines = src.splitlines()
    found = []
    for i, line in enumerate(lines):
        m = re.match(r"var (\w+Schema) = \[\]string\{", line)
        if not m:
            continue
        for j in range(i + 1, len(lines)):
            if lines[j] == "}":
                found.append((m.group(1), i + 1, j))
                break
    return lines, found


def main():
    if not GO.exists():
        sys.exit(f"cannot find {GO}")
    src = GO.read_text()
    lines, found = groups(src)
    if not found:
        sys.exit("no schema blocks found; did db.go change shape?")

    out = [HEADER]
    total = 0
    for name, start, end in found:
        body = "\n".join(lines[start:end])
        stmts = re.findall(r"`([^`]*)`", body)
        total += len(stmts)
        out.append(f"const {name}* = [\n")
        for st in stmts:
            if '"""' in st:
                sys.exit(f"statement contains a triple quote: {st[:60]}")
            out.append(f'  """{st}""",\n')
        out.append("]\n\n")
        print(f"{name}: {len(stmts)} statements")

    OUT.write_text("".join(out))
    print(f"wrote {OUT} ({total} statements)")


if __name__ == "__main__":
    main()
