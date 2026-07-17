#!/usr/bin/env python3
"""
Fixes "cannot scan timestamptz into **string" by changing ESignField's
FilledAt field from *string to *time.Time, matching the actual Postgres
column type.
Run from ~/scp-hub-go. Usage: python3 fix_filledat_type.py
"""
import sys

PATH = "internal/repository/esign_fields_repository.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# ── 1. Add "time" to imports ──────────────────────────────────────────────────
old_imports = (
    'import (\n'
    '\t"context"\n'
    '\t"fmt"\n'
    '\n'
    '\t"github.com/google/uuid"\n'
    '\t"github.com/jackc/pgx/v5/pgxpool"\n'
    ')\n'
)
count = src.count(old_imports)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the import block, found {count}. Aborting.")
    sys.exit(1)

new_imports = (
    'import (\n'
    '\t"context"\n'
    '\t"fmt"\n'
    '\t"time"\n'
    '\n'
    '\t"github.com/google/uuid"\n'
    '\t"github.com/jackc/pgx/v5/pgxpool"\n'
    ')\n'
)
src = src.replace(old_imports, new_imports, 1)

# ── 2. Fix the struct field type ──────────────────────────────────────────────
old_field = '\tFilledAt    *string `json:"filled_at"`\n'
count2 = src.count(old_field)
if count2 != 1:
    print(f"ERROR: expected exactly 1 occurrence of the FilledAt struct field, found {count2}. Aborting.")
    sys.exit(1)

new_field = '\tFilledAt    *time.Time `json:"filled_at"`\n'
src = src.replace(old_field, new_field, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — FilledAt changed from *string to *time.Time, time import added.")
print("Next: go build ./...")
