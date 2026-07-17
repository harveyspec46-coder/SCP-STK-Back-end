#!/usr/bin/env python3
"""
Adds user_id to each signer object sent by createDocument, linking the
signer row back to the actual selected user's login.
Run from ~/scp-final. Usage: python3 apply_signer_userid_frontend.py
"""
import sys

PATH = "src/App.jsx"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

anchor = (
    '      const signers = selectedSignerIds.map((id) => {\n'
    '        const u = boardUsers.find((x) => x.id === id);\n'
    '        return { name: u.full_name, email: u.email, role: u.role };\n'
    '      });\n'
)

count = src.count(anchor)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the signers-mapping block, found {count}. Aborting.")
    sys.exit(1)

replacement = (
    '      const signers = selectedSignerIds.map((id) => {\n'
    '        const u = boardUsers.find((x) => x.id === id);\n'
    '        return { name: u.full_name, email: u.email, role: u.role, user_id: u.id };\n'
    '      });\n'
)

src = src.replace(anchor, replacement, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — signers now include user_id.")
print("Next: npm run build")
