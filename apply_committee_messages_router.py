#!/usr/bin/env python3
"""
Adds CommitteeMessages to the Handlers struct and registers its routes
inside the existing /committees route group.
Run from ~/scp-hub-go. Usage: python3 apply_committee_messages_router.py
"""
import sys

PATH = "internal/router/router.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

struct_anchor = "\tCommitteeDocuments *handler.CommitteeDocumentHandler\n"
count = src.count(struct_anchor)
if count != 1:
    print("ERROR: expected exactly 1 occurrence of the CommitteeDocuments struct field, found " + str(count) + ". Aborting.")
    sys.exit(1)

src = src.replace(
    struct_anchor,
    struct_anchor + "\tCommitteeMessages  *handler.CommitteeMessageHandler\n",
    1,
)

route_anchor = 'r.Delete("/{id}/documents/{docId}", h.CommitteeDocuments.Delete)\n'
count2 = src.count(route_anchor)
if count2 != 1:
    print("ERROR: expected exactly 1 occurrence of the CommitteeDocuments Delete route line, found " + str(count2) + ". Aborting.")
    sys.exit(1)

new_routes = (
    '\t\t\tr.Get("/{id}/messages", h.CommitteeMessages.List)\n'
    '\t\t\tr.Post("/{id}/messages", h.CommitteeMessages.Send)\n'
)

src = src.replace(route_anchor, route_anchor + new_routes, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success -- CommitteeMessages added to Handlers struct and routed.")
print("Next: run apply_committee_messages_main_wiring.py, then go build ./...")
