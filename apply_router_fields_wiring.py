#!/usr/bin/env python3
"""
Adds ESignFields to the Handlers struct and registers the field routes.
Run from ~/scp-hub-go. Usage: python3 apply_router_fields_wiring.py
"""
import re
import sys

PATH = "internal/router/router.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# ── 1. Add ESignFields field to the Handlers struct ───────────────────────────
struct_anchor = "\tESign         *handler.ESignHandler\n"
count = src.count(struct_anchor)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the ESign struct field line, found {count}. Aborting.")
    sys.exit(1)

src = src.replace(
    struct_anchor,
    struct_anchor + "\tESignFields   *handler.ESignFieldsHandler\n",
    1,
)

# ── 2. Register the new routes right after the existing esign Create route ───
route_anchor_pattern = re.compile(
    r'(r\.Post\("/documents",\s*h\.ESign\.Create\)\s*\n)'
)
matches = route_anchor_pattern.findall(src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of the esign Create route line, found {len(matches)}. Aborting.")
    sys.exit(1)

new_routes = (
    '\t\tr.Get("/documents/{id}/fields", h.ESignFields.List)\n'
    '\t\tr.Post("/documents/{id}/fields", h.ESignFields.Create)\n'
    '\t\tr.Patch("/fields/{id}", h.ESignFields.Fill)\n'
)

src = route_anchor_pattern.sub(lambda m: m.group(1) + new_routes, src, count=1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — ESignFields added to Handlers struct, 3 new routes registered.")
print("Note: if this file already has a my-signature route block inserted right after")
print("the Create route (from an earlier session), double-check the ordering with:")
print('  grep -n "esign\\|my-signature\\|fields" internal/router/router.go')
