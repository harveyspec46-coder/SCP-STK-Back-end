#!/usr/bin/env python3
"""
Fixes duplicate "Committees" struct field and duplicate committee route
blocks caused by running apply_committee_router.py more than once.
Run from ~/scp-hub-go. Usage: python3 fix_duplicate_committees.py
"""
import sys

PATH = "internal/router/router.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# 1. Dedupe the struct field
struct_field = "\tCommittees    *handler.CommitteeHandler\n"
count1 = src.count(struct_field)
if count1 > 1:
    src = src.replace(struct_field, "", count1 - 1)
    print("Removed " + str(count1 - 1) + " duplicate struct field line(s).")
elif count1 == 1:
    print("Struct field: only 1 occurrence found, nothing to dedupe.")
else:
    print("WARNING: struct field not found at all -- something else may be wrong.")

# 2. Dedupe the route block
route_block = (
    "\n\n\t\t// -- Committees -- board can view, admin manages --\n"
    "\t\tr.Route(\"/committees\", func(r chi.Router) {\n"
    "\t\t\tr.Use(auth.RequireRole(\"manager\"))\n"
    "\t\t\tr.Get(\"/\", h.Committees.List)\n"
    "\t\t\tr.With(auth.RequireExactRole(\"admin\")).Post(\"/\", h.Committees.Create)\n"
    "\t\t\tr.With(auth.RequireExactRole(\"admin\")).Delete(\"/{id}\", h.Committees.Delete)\n"
    "\t\t\tr.With(auth.RequireExactRole(\"admin\")).Post(\"/{id}/members\", h.Committees.AddMember)\n"
    "\t\t\tr.With(auth.RequireExactRole(\"admin\")).Patch(\"/{id}/members/{memberId}\", h.Committees.UpdateMemberRole)\n"
    "\t\t\tr.With(auth.RequireExactRole(\"admin\")).Delete(\"/{id}/members/{memberId}\", h.Committees.RemoveMember)\n"
    "\t\t})"
)
count2 = src.count(route_block)
if count2 > 1:
    src = src.replace(route_block, "", count2 - 1)
    print("Removed " + str(count2 - 1) + " duplicate route block(s).")
elif count2 == 1:
    print("Route block: only 1 occurrence found, nothing to dedupe.")
else:
    print("WARNING: route block not found at all -- something else may be wrong.")

if src == original:
    print("No changes were made -- file may already be clean, or the exact text didn't match.")
else:
    with open(PATH, "w", encoding="utf-8") as f:
        f.write(src)
    print("File updated.")

print("Next: go build ./...")
