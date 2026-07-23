#!/usr/bin/env python3
"""
Adds CommitteeDocuments to the Handlers struct and registers its routes
inside the existing /committees route group.
Run from ~/scp-hub-go. Usage: python3 apply_committee_docs_router.py
"""
import sys

PATH = "internal/router/router.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# 1. Add struct field
struct_anchor = "\tCommittees    *handler.CommitteeHandler\n"
count = src.count(struct_anchor)
if count != 1:
    print("ERROR: expected exactly 1 occurrence of the Committees struct field, found " + str(count) + ". Aborting.")
    sys.exit(1)

src = src.replace(
    struct_anchor,
    struct_anchor + "\tCommitteeDocuments *handler.CommitteeDocumentHandler\n",
    1,
)

# 2. Add routes inside the existing /committees group, right after the
# RemoveMember route line.
route_anchor = 'r.With(auth.RequireExactRole("admin")).Delete("/{id}/members/{memberId}", h.Committees.RemoveMember)\n'
count2 = src.count(route_anchor)
if count2 != 1:
    print("ERROR: expected exactly 1 occurrence of the RemoveMember route line, found " + str(count2) + ". Aborting.")
    sys.exit(1)

new_routes = (
    '\t\t\tr.Get("/{id}/documents", h.CommitteeDocuments.List)\n'
    '\t\t\tr.Post("/{id}/documents", h.CommitteeDocuments.Add)\n'
    '\t\t\tr.Delete("/{id}/documents/{docId}", h.CommitteeDocuments.Delete)\n'
)

src = src.replace(route_anchor, route_anchor + new_routes, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success -- CommitteeDocuments added to Handlers struct and routed.")
print("Next: run apply_committee_docs_main_wiring.py, then go build ./...")
