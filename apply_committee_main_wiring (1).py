#!/usr/bin/env python3
"""
Constructs committeeRepo/committeeHandler in main.go and adds Committees to
the Handlers{...} literal.
Run from ~/scp-hub-go. Usage: python3 apply_committee_main_wiring.py
"""
import re
import sys

PATH = "cmd/server/main.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# 1. Add committeeRepo creation right after esignFieldsRepo is created
anchor_pattern = re.compile(r'([ \t]*esignFieldsRepo\s*:=\s*repository\.NewESignFieldsRepo\(pool\)\s*\n)')
matches = anchor_pattern.findall(src)
if len(matches) != 1:
    print("ERROR: expected exactly 1 occurrence of the esignFieldsRepo line, found " + str(len(matches)) + ". Aborting.")
    sys.exit(1)

indent_match = re.search(r'([ \t]*)esignFieldsRepo\s*:=\s*repository\.NewESignFieldsRepo', src)
indent = indent_match.group(1) if indent_match else "\t"

src = anchor_pattern.sub(
    lambda m: m.group(1) + indent + "committeeRepo := repository.NewCommitteeRepo(pool)\n",
    src,
    count=1,
)

# 2. Add Committees: handler.NewCommitteeHandler(...) to Handlers{...}
handlers_line_pattern = re.compile(
    r'([ \t]*ESignFields:\s*handler\.NewESignFieldsHandler\(esignFieldsRepo,\s*esignRepo,\s*auditRepo\),\s*\n)'
)
matches2 = handlers_line_pattern.findall(src)
if len(matches2) != 1:
    print("ERROR: expected exactly 1 occurrence of the ESignFields: handler line, found " + str(len(matches2)) + ". Aborting.")
    sys.exit(1)

indent_match2 = re.search(r'([ \t]*)ESignFields:\s*handler\.NewESignFieldsHandler', src)
indent2 = indent_match2.group(1) if indent_match2 else "\t\t"

src = handlers_line_pattern.sub(
    lambda m: m.group(1) + indent2 + "Committees:    handler.NewCommitteeHandler(committeeRepo, auditRepo),\n",
    src,
    count=1,
)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success -- committeeRepo constructed and Committees wired into Handlers{...}.")
print("Next: go build ./...")
