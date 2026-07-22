#!/usr/bin/env python3
"""
Adds Committees to the Handlers struct and registers its routes
(list/create board-wide readable, create/delete/member-management admin-only).
Run from ~/scp-hub-go. Usage: python3 apply_committee_router.py
"""
import sys

PATH = "internal/router/router.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

# 1. Add Committees field to Handlers struct
struct_anchor = "\tESignFields   *handler.ESignFieldsHandler\n"
count = src.count(struct_anchor)
if count != 1:
    print("ERROR: expected exactly 1 occurrence of the ESignFields struct field, found " + str(count) + ". Aborting.")
    sys.exit(1)

src = src.replace(
    struct_anchor,
    struct_anchor + "\tCommittees    *handler.CommitteeHandler\n",
    1,
)

# 2. Register routes -- board can list, admin-only for everything else.
# Insert as a new route group right after the esign group closes. We find
# the esign group's closing "})" by anchoring on the finalize route we
# added earlier in the same group, then finding the next "})" after it.
anchor_line = 'r.Post("/documents/{id}/signers/{signerId}/finalize", h.ESignFields.Finalize)\n'
count2 = src.count(anchor_line)
if count2 != 1:
    print("ERROR: expected exactly 1 occurrence of the finalize route line, found " + str(count2) + ". Aborting.")
    sys.exit(1)

idx = src.index(anchor_line) + len(anchor_line)
close_idx = src.index("})", idx)
insert_at = close_idx + len("})")

committee_routes = (
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

src = src[:insert_at] + committee_routes + src[insert_at:]

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success -- Committees added to Handlers struct and routed.")
print("Next: run apply_committee_main_wiring.py, then go build ./...")
