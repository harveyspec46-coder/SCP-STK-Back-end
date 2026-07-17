#!/usr/bin/env python3
"""
Adds a UserID field to ESignSignerRequest so signers can be linked back to
their actual login, needed for "is this my turn to sign" matching.
Run from ~/scp-hub-go. Usage: python3 apply_signer_userid_model.py
"""
import sys

PATH = "internal/model/model_extended.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

anchor = (
    'type ESignSignerRequest struct {\n'
    '\tName  string `json:"name"`\n'
    '\tEmail string `json:"email"`\n'
    '\tRole  string `json:"role,omitempty"` // admin | manager | staff | participant | external — defaults to "external"\n'
)

count = src.count(anchor)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the ESignSignerRequest struct start, found {count}. Aborting.")
    sys.exit(1)

replacement = (
    'type ESignSignerRequest struct {\n'
    '\tName   string `json:"name"`\n'
    '\tEmail  string `json:"email"`\n'
    '\tRole   string `json:"role,omitempty"` // admin | manager | staff | participant | external — defaults to "external"\n'
    '\tUserID string `json:"user_id,omitempty"` // links this signer row back to their actual login, for "is this my turn" matching\n'
)

src = src.replace(anchor, replacement, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — UserID added to ESignSignerRequest.")
