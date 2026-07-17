#!/usr/bin/env python3
"""
Adds POST /api/esign/documents/{id}/signers/{signerId}/finalize — an
idempotent check that marks a signer (and possibly the whole document)
complete if all their fields are filled. This closes a real gap: the
completion side effect previously only ran inside Fill(), so a signer whose
fields were already filled in an earlier session (interrupted, or a UI
error that masked an actual success) never got marked complete.
Run from ~/scp-hub-go. Usage: python3 apply_finalize_endpoint.py
"""
import re
import sys

# ── 1. Add Finalize method to the handler ─────────────────────────────────────
HANDLER_PATH = "internal/handler/esign_fields_handler.go"
with open(HANDLER_PATH, "r", encoding="utf-8") as f:
    handler_src = f.read()
handler_original = handler_src

anchor = '''// PATCH /api/esign/fields/{id}'''
count = handler_src.count(anchor)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the Fill handler doc comment, found {count}. Aborting.")
    sys.exit(1)

finalize_method = '''// POST /api/esign/documents/{id}/signers/{signerId}/finalize
// Idempotent: checks whether every field belonging to this signer on this
// document is filled, and if so marks the signer (and possibly the whole
// document) complete. Safe to call even if already complete, or if fields
// were filled in an earlier session — this is the only place that
// guarantees completion status gets corrected regardless of how the fields
// ended up filled.
func (h *ESignFieldsHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	signerID := chi.URLParam(r, "signerId")

	allDone, err := h.fields.AllFilledForSigner(r.Context(), docID, signerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check field completion")
		return
	}
	if !allDone {
		writeJSON(w, http.StatusOK, model.Response{Message: "not all fields filled yet", Data: map[string]bool{"complete": false}})
		return
	}

	if err := h.signers.MarkSignerComplete(r.Context(), docID, signerID); err != nil {
		log.Printf("esign fields finalize: failed to mark signer complete: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to finalize signer")
		return
	}

	writeJSON(w, http.StatusOK, model.Response{Message: "signer finalized", Data: map[string]bool{"complete": true}})
}

// PATCH /api/esign/fields/{id}'''

handler_src = handler_src.replace(anchor, finalize_method, 1)

if handler_src == handler_original:
    print("ERROR: no changes made to handler (unexpected). Aborting.")
    sys.exit(1)

with open(HANDLER_PATH, "w", encoding="utf-8") as f:
    f.write(handler_src)
print(f"Updated {HANDLER_PATH}")

# ── 2. Register the route in router.go ────────────────────────────────────────
ROUTER_PATH = "internal/router/router.go"
with open(ROUTER_PATH, "r", encoding="utf-8") as f:
    router_src = f.read()
router_original = router_src

route_anchor_pattern = re.compile(r'(r\.Patch\("/fields/\{id\}",\s*h\.ESignFields\.Fill\)\s*\n)')
matches = route_anchor_pattern.findall(router_src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of the fields Fill route line, found {len(matches)}. Aborting.")
    sys.exit(1)

new_route = '\t\tr.Post("/documents/{id}/signers/{signerId}/finalize", h.ESignFields.Finalize)\n'
router_src = route_anchor_pattern.sub(lambda m: m.group(1) + new_route, router_src, count=1)

if router_src == router_original:
    print("ERROR: no changes made to router (unexpected). Aborting.")
    sys.exit(1)

with open(ROUTER_PATH, "w", encoding="utf-8") as f:
    f.write(router_src)
print(f"Updated {ROUTER_PATH}")

print("Success — Finalize endpoint added and routed.")
print("Next: go build ./...")
