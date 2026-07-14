#!/usr/bin/env python3
"""
Wires SignatureRepo into main.go's ESignHandler construction.
Run from ~/scp-hub-go. Usage: python3 apply_main_signature_wiring.py
"""
import re
import sys

PATH = "cmd/server/main.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# ── 1. Add sigRepo creation right after esignRepo is created ─────────────────
esign_repo_pattern = re.compile(
    r'([ \t]*esignRepo\s*:=\s*repository\.NewESignRepo\(pool,\s*ilovepdfClient,\s*esignStorage\)\s*\n)'
)
matches = esign_repo_pattern.findall(src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of the esignRepo := repository.NewESignRepo(...) line, found {len(matches)}. Aborting.")
    sys.exit(1)

# Capture the exact leading whitespace so the new line matches indentation style
indent_match = re.search(r'([ \t]*)esignRepo\s*:=\s*repository\.NewESignRepo', src)
indent = indent_match.group(1) if indent_match else "\t"

src = esign_repo_pattern.sub(
    r'\1' + indent + 'sigRepo := repository.NewSignatureRepo(pool)\n',
    src,
    count=1,
)

# ── 2. Update NewESignHandler(esignRepo, auditRepo) call to pass sigRepo too ─
handler_call_pattern = re.compile(
    r'handler\.NewESignHandler\(esignRepo,\s*auditRepo\)'
)
matches = handler_call_pattern.findall(src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of handler.NewESignHandler(esignRepo, auditRepo), found {len(matches)}. Aborting.")
    sys.exit(1)

src = handler_call_pattern.sub(
    'handler.NewESignHandler(esignRepo, sigRepo, auditRepo)',
    src,
    count=1,
)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — main.go updated: sigRepo created and passed into NewESignHandler.")
print("Next: go build ./...")
