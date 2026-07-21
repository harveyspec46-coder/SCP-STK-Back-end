#!/usr/bin/env python3
"""
Adds DELETE /api/esign/documents/{id} -- checks the requester created the
document, then deletes it (cascades to signers/fields via existing FK
ON DELETE CASCADE constraints).
Run from ~/scp-hub-go. Usage: python3 apply_delete_document.py
"""
import re
import sys

REPO_PATH = "internal/repository/esign.go"
with open(REPO_PATH, "r", encoding="utf-8") as f:
    repo_src = f.read()
repo_original = repo_src

anchor = "// HandleSignerEvent updates one signer's status from a webhook event"
count = repo_src.count(anchor)
if count != 1:
    print("ERROR: expected exactly 1 occurrence of the HandleSignerEvent doc comment, found " + str(count) + ". Aborting.")
    sys.exit(1)

delete_method = (
    '// Delete removes a document (and, via ON DELETE CASCADE, its signers and\n'
    '// fields) -- but only if requestedBy matches the document\'s created_by.\n'
    '// Returns a sentinel error the handler checks for to return 403 vs 500.\n'
    'var ErrNotDocumentOwner = fmt.Errorf("only the document\'s creator can delete it")\n'
    '\n'
    'func (r *ESignRepo) Delete(ctx context.Context, documentID, requestedBy string) error {\n'
    '\tvar createdBy string\n'
    '\terr := r.db.QueryRow(ctx,\n'
    '\t\t`SELECT created_by FROM esign_documents WHERE id = $1`, documentID).Scan(&createdBy)\n'
    '\tif err != nil {\n'
    '\t\tif err == pgx.ErrNoRows {\n'
    '\t\t\treturn fmt.Errorf("document not found")\n'
    '\t\t}\n'
    '\t\treturn fmt.Errorf("lookup document owner: %w", err)\n'
    '\t}\n'
    '\tif createdBy != requestedBy {\n'
    '\t\treturn ErrNotDocumentOwner\n'
    '\t}\n'
    '\n'
    '\tif _, err := r.db.Exec(ctx, `DELETE FROM esign_documents WHERE id = $1`, documentID); err != nil {\n'
    '\t\treturn fmt.Errorf("delete document: %w", err)\n'
    '\t}\n'
    '\treturn nil\n'
    '}\n'
    '\n'
    '// HandleSignerEvent updates one signer\'s status from a webhook event'
)

repo_src = repo_src.replace(anchor, delete_method, 1)

if repo_src == repo_original:
    print("ERROR: no changes made to esign.go (unexpected). Aborting.")
    sys.exit(1)

with open(REPO_PATH, "w", encoding="utf-8") as f:
    f.write(repo_src)
print("Updated " + REPO_PATH)

HANDLER_PATH = "internal/handler/esign.go"
with open(HANDLER_PATH, "r", encoding="utf-8") as f:
    handler_src = f.read()
handler_original = handler_src

handler_anchor = "// GET /api/esign/my-signature"
count2 = handler_src.count(handler_anchor)
if count2 != 1:
    print("ERROR: expected exactly 1 occurrence of the GET my-signature doc comment, found " + str(count2) + ". Aborting.")
    sys.exit(1)

delete_handler = (
    '// DELETE /api/esign/documents/{id}\n'
    '// Only the document\'s creator can delete it.\n'
    'func (h *ESignHandler) Delete(w http.ResponseWriter, r *http.Request) {\n'
    '\tdocID := chi.URLParam(r, "id")\n'
    '\tuser := auth.GetUser(r.Context())\n'
    '\n'
    '\terr := h.repo.Delete(r.Context(), docID, user.ID)\n'
    '\tif err != nil {\n'
    '\t\tif err == repository.ErrNotDocumentOwner {\n'
    '\t\t\twriteError(w, http.StatusForbidden, "only the document\'s creator can delete it")\n'
    '\t\t\treturn\n'
    '\t\t}\n'
    '\t\tlog.Printf("esign delete failed: %v", err)\n'
    '\t\twriteError(w, http.StatusInternalServerError, "failed to delete document")\n'
    '\t\treturn\n'
    '\t}\n'
    '\n'
    '\t_ = h.audit.Record(r.Context(), user.ID, "esign_document_deleted", "E-Signatures",\n'
    '\t\t"Deleted document "+docID, r.RemoteAddr)\n'
    '\twriteJSON(w, http.StatusOK, model.Response{Message: "document deleted"})\n'
    '}\n'
    '\n'
    '// GET /api/esign/my-signature'
)

handler_src = handler_src.replace(handler_anchor, delete_handler, 1)

if handler_src == handler_original:
    print("ERROR: no changes made to esign.go handler (unexpected). Aborting.")
    sys.exit(1)

if '"github.com/go-chi/chi/v5"' not in handler_src:
    import_anchor = '"github.com/scp-stk/hub/internal/auth"'
    if handler_src.count(import_anchor) != 1:
        print("ERROR: could not find import block to add chi import. Aborting.")
        sys.exit(1)
    handler_src = handler_src.replace(
        import_anchor,
        '"github.com/go-chi/chi/v5"\n\n\t"github.com/scp-stk/hub/internal/auth"',
        1,
    )
    print("Note: added chi/v5 import to esign.go handler (was missing).")

with open(HANDLER_PATH, "w", encoding="utf-8") as f:
    f.write(handler_src)
print("Updated " + HANDLER_PATH)

ROUTER_PATH = "internal/router/router.go"
with open(ROUTER_PATH, "r", encoding="utf-8") as f:
    router_src = f.read()
router_original = router_src

route_anchor_pattern = re.compile(r'(r\.Post\("/documents",\s*h\.ESign\.Create\)\s*\n)')
matches = route_anchor_pattern.findall(router_src)
if len(matches) != 1:
    print("ERROR: expected exactly 1 occurrence of the esign Create route line, found " + str(len(matches)) + ". Aborting.")
    sys.exit(1)

new_route = '\t\tr.Delete("/documents/{id}", h.ESign.Delete)\n'
router_src = route_anchor_pattern.sub(lambda m: m.group(1) + new_route, router_src, count=1)

if router_src == router_original:
    print("ERROR: no changes made to router.go (unexpected). Aborting.")
    sys.exit(1)

with open(ROUTER_PATH, "w", encoding="utf-8") as f:
    f.write(router_src)
print("Updated " + ROUTER_PATH)

print("Success -- delete endpoint added (repo + handler + route).")
print("Next: go build ./...")
