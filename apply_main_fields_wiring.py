#!/usr/bin/env python3
"""
Constructs esignFieldsRepo/esignFieldsHandler in main.go and adds
ESignFields to the Handlers{...} literal.
Run from ~/scp-hub-go. Usage: python3 apply_main_fields_wiring.py
"""
import re
import sys

PATH = "cmd/server/main.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# ── 1. Add esignFieldsRepo creation right after sigRepo is created ───────────
sig_repo_pattern = re.compile(r'([ \t]*sigRepo\s*:=\s*repository\.NewSignatureRepo\(pool\)\s*\n)')
matches = sig_repo_pattern.findall(src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of the sigRepo := repository.NewSignatureRepo(pool) line, found {len(matches)}. Aborting.")
    sys.exit(1)

indent_match = re.search(r'([ \t]*)sigRepo\s*:=\s*repository\.NewSignatureRepo', src)
indent = indent_match.group(1) if indent_match else "\t"

src = sig_repo_pattern.sub(
    r'\1' + indent + 'esignFieldsRepo := repository.NewESignFieldsRepo(pool)\n',
    src,
    count=1,
)

# ── 2. Add ESignFields: handler.NewESignFieldsHandler(...) to Handlers{...} ──
handlers_line_pattern = re.compile(
    r'([ \t]*ESign:\s*handler\.NewESignHandler\(esignRepo,\s*sigRepo,\s*auditRepo\),\s*\n)'
)
matches = handlers_line_pattern.findall(src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of the ESign: handler.NewESignHandler(...) line, found {len(matches)}. Aborting.")
    sys.exit(1)

indent_match2 = re.search(r'([ \t]*)ESign:\s*handler\.NewESignHandler', src)
indent2 = indent_match2.group(1) if indent_match2 else "\t\t"

src = handlers_line_pattern.sub(
    lambda m: m.group(1) + indent2 + 'ESignFields:   handler.NewESignFieldsHandler(esignFieldsRepo, esignRepo, auditRepo),\n',
    src,
    count=1,
)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — esignFieldsRepo constructed and ESignFields wired into Handlers{...}.")
print("Next: go build ./...")
