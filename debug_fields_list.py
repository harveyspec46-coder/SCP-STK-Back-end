#!/usr/bin/env python3
"""
Adds log.Printf to ESignFieldsHandler.List so the real DB/scan error shows
up in Railway logs instead of a silent generic 500.
Run from ~/scp-hub-go. Usage: python3 debug_fields_list.py
"""
import sys

PATH = "internal/handler/esign_fields_handler.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

anchor = (
    '\tfields, err := h.fields.ListForDocument(r.Context(), docID)\n'
    '\tif err != nil {\n'
    '\t\twriteError(w, http.StatusInternalServerError, "failed to list fields")\n'
    '\t\treturn\n'
    '\t}\n'
)

count = src.count(anchor)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the List handler body, found {count}. Aborting.")
    sys.exit(1)

replacement = (
    '\tfields, err := h.fields.ListForDocument(r.Context(), docID)\n'
    '\tif err != nil {\n'
    '\t\tlog.Printf("esign fields list failed: %v", err)\n'
    '\t\twriteError(w, http.StatusInternalServerError, "failed to list fields")\n'
    '\t\treturn\n'
    '\t}\n'
)

src = src.replace(anchor, replacement, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — logging added to List handler.")
print("Next: go build ./... (may need to add \"log\" to imports if not already present)")
