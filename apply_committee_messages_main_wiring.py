#!/usr/bin/env python3
"""
Constructs committeeMessageRepo/handler in main.go and adds
CommitteeMessages to the Handlers{...} literal.
Run from ~/scp-hub-go. Usage: python3 apply_committee_messages_main_wiring.py
"""
import re
import sys

PATH = "cmd/server/main.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

anchor_pattern = re.compile(r'([ \t]*committeeDocumentRepo\s*:=\s*repository\.NewCommitteeDocumentRepo\(pool\)\s*\n)')
matches = anchor_pattern.findall(src)
if len(matches) != 1:
    print("ERROR: expected exactly 1 occurrence of the committeeDocumentRepo line, found " + str(len(matches)) + ". Aborting.")
    sys.exit(1)

indent_match = re.search(r'([ \t]*)committeeDocumentRepo\s*:=\s*repository\.NewCommitteeDocumentRepo', src)
indent = indent_match.group(1) if indent_match else "\t"

src = anchor_pattern.sub(
    lambda m: m.group(1) + indent + "committeeMessageRepo := repository.NewCommitteeMessageRepo(pool)\n",
    src,
    count=1,
)

handlers_line_pattern = re.compile(
    r'([ \t]*CommitteeDocuments:\s*handler\.NewCommitteeDocumentHandler\(committeeDocumentRepo,\s*auditRepo\),\s*\n)'
)
matches2 = handlers_line_pattern.findall(src)
if len(matches2) != 1:
    print("ERROR: expected exactly 1 occurrence of the CommitteeDocuments: handler line, found " + str(len(matches2)) + ". Aborting.")
    sys.exit(1)

indent_match2 = re.search(r'([ \t]*)CommitteeDocuments:\s*handler\.NewCommitteeDocumentHandler', src)
indent2 = indent_match2.group(1) if indent_match2 else "\t\t"

src = handlers_line_pattern.sub(
    lambda m: m.group(1) + indent2 + "CommitteeMessages:  handler.NewCommitteeMessageHandler(committeeMessageRepo),\n",
    src,
    count=1,
)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success -- committeeMessageRepo constructed and CommitteeMessages wired into Handlers{...}.")
print("Next: go build ./...")
