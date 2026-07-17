#!/usr/bin/env python3
"""
Updates the signer INSERT in Create() to also store user_id, so a signer
row can be matched back to the actual logged-in user later.
Run from ~/scp-hub-go. Usage: python3 apply_signer_userid_insert.py
"""
import sys

PATH = "internal/repository/esign.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

anchor = (
    '\tfor _, s := range req.Signers {\n'
    '\t\trole := s.Role\n'
    '\t\tif role == "" {\n'
    '\t\t\trole = "external"\n'
    '\t\t}\n'
    '\t\tsid := uuid.New().String()\n'
    '\t\tif _, err := tx.Exec(ctx,\n'
    '\t\t\t`INSERT INTO esign_signers (id, document_id, name, email, role, signed)\n'
    '\t\t\t VALUES ($1,$2,$3,$4,$5,false)`,\n'
    '\t\t\tsid, id, s.Name, s.Email, role); err != nil {\n'
    '\t\t\treturn nil, fmt.Errorf("create signer %q: %w", s.Email, err)\n'
    '\t\t}\n'
    '\t}\n'
)

count = src.count(anchor)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the signer INSERT loop, found {count}. Aborting.")
    sys.exit(1)

replacement = (
    '\tfor _, s := range req.Signers {\n'
    '\t\trole := s.Role\n'
    '\t\tif role == "" {\n'
    '\t\t\trole = "external"\n'
    '\t\t}\n'
    '\t\tsid := uuid.New().String()\n'
    '\t\tvar userID interface{}\n'
    '\t\tif s.UserID != "" {\n'
    '\t\t\tuserID = s.UserID\n'
    '\t\t}\n'
    '\t\tif _, err := tx.Exec(ctx,\n'
    '\t\t\t`INSERT INTO esign_signers (id, document_id, name, email, role, user_id, signed)\n'
    '\t\t\t VALUES ($1,$2,$3,$4,$5,$6,false)`,\n'
    '\t\t\tsid, id, s.Name, s.Email, role, userID); err != nil {\n'
    '\t\t\treturn nil, fmt.Errorf("create signer %q: %w", s.Email, err)\n'
    '\t\t}\n'
    '\t}\n'
)

src = src.replace(anchor, replacement, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — user_id now stored on each signer at creation time.")
