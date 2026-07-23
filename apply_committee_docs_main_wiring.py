#!/usr/bin/env python3
"""
Constructs committeeDocumentRepo/handler in main.go and adds
CommitteeDocuments to the Handlers{...} literal.
Run from ~/scp-hub-go. Usage: python3 apply_committee_docs_main_wiring.py
"""
import re
import sys

PATH = "cmd/server/main.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# 1. Add committeeDocumentRepo creation right after committeeRepo is created
anchor_pattern = re.compile(r'([ \t]*committeeRepo\s*:=\s*repository\.NewCommitteeRepo\(pool\)\s*\n)')
matches = anchor_pattern.findall(src)
if len(matches) != 1:
    print("ERROR: expected exactly 1 occurrence of the committeeRepo line, found " + str(len(matches)) + ". Aborting.")
    sys.exit(1)

indent_match = re.search(r'([ \t]*)committeeRepo\s*:=\s*repository\.NewCommitteeRepo', src)
indent = indent_match.group(1) if indent_match else "\t"

src = anchor_pattern.sub(
    lambda m: m.group(1) + indent + "committeeDocumentRepo := repository.NewCommitteeDocumentRepo(pool)\n",
    src,
    count=1,
)

# 2. Add CommitteeDocuments: handler.NewCommitteeDocumentHandler(...) to Handlers{...}
handlers_line_pattern = re.compile(
    r'([ \t]*Committees:\s*handler\.NewCommitteeHandler\(committeeRepo,\s*auditRepo\),\s*\n)'
)
matches2 = handlers_line_pattern.findall(src)
if len(matches2) != 1:
    print("ERROR: expected exactly 1 occurrence of the Committees: handler line, found " + str(len(matches2)) + ". Aborting.")
    sys.exit(1)

indent_match2 = re.search(r'([ \t]*)Committees:\s*handler\.NewCommitteeHandler', src)
indent2 = indent_match2.group(1) if indent_match2 else "\t\t"

src = handlers_line_pattern.sub(
    lambda m: m.group(1) + indent2 + "CommitteeDocuments: handler.NewCommitteeDocumentHandler(committeeDocumentRepo, auditRepo),\n",
    src,
    count=1,
)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success -- committeeDocumentRepo constructed and CommitteeDocuments wired into Handlers{...}.")
print("Next: go build ./...")
