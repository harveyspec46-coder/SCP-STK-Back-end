#!/usr/bin/env python3
"""
Adds the "my-signature" backend endpoints.
Run from ~/scp-hub-go. Usage: python3 apply_my_signature_backend.py
"""
import re
import sys

# ── 1. New repository file ─────────────────────────────────────────────────
SIGNATURE_REPO_PATH = "internal/repository/signature.go"

signature_repo_content = '''package repository

import (
\t"context"
\t"fmt"

\t"github.com/jackc/pgx/v5"
\t"github.com/jackc/pgx/v5/pgxpool"
)

// ════════════════════════════════════════════════════════════════════════════
// User-level saved signatures — set up once per admin/manager, then reused
// on every document they sign. Separate from esign_documents/esign_signers,
// which track per-document state.
// ════════════════════════════════════════════════════════════════════════════

type UserSignature struct {
\tUserID         string `json:"user_id"`
\tFullName       string `json:"full_name"`
\tFontStyle      string `json:"font_style"`
\tSignatureImage string `json:"signature_image"`
}

type SignatureRepo struct {
\tdb *pgxpool.Pool
}

func NewSignatureRepo(db *pgxpool.Pool) *SignatureRepo {
\treturn &SignatureRepo{db: db}
}

// GetMine returns the caller's saved signature, or nil if they haven't set
// one up yet (not an error — the frontend uses this to decide whether to
// show the "set up your signature" screen).
func (r *SignatureRepo) GetMine(ctx context.Context, userID string) (*UserSignature, error) {
\tvar s UserSignature
\terr := r.db.QueryRow(ctx,
\t\t`SELECT user_id, full_name, font_style, signature_image
\t\t FROM user_signatures WHERE user_id = $1`, userID).
\t\tScan(&s.UserID, &s.FullName, &s.FontStyle, &s.SignatureImage)
\tif err != nil {
\t\tif err == pgx.ErrNoRows {
\t\t\treturn nil, nil
\t\t}
\t\treturn nil, fmt.Errorf("get user signature: %w", err)
\t}
\treturn &s, nil
}

// SaveMine creates or overwrites the caller's saved signature — matches the
// "set it up once" design; there's intentionally no history of prior styles.
func (r *SignatureRepo) SaveMine(ctx context.Context, userID, fullName, fontStyle, signatureImage string) (*UserSignature, error) {
\tvar s UserSignature
\terr := r.db.QueryRow(ctx,
\t\t`INSERT INTO user_signatures (user_id, full_name, font_style, signature_image, updated_at)
\t\t VALUES ($1, $2, $3, $4, NOW())
\t\t ON CONFLICT (user_id) DO UPDATE
\t\t   SET full_name = $2, font_style = $3, signature_image = $4, updated_at = NOW()
\t\t RETURNING user_id, full_name, font_style, signature_image`,
\t\tuserID, fullName, fontStyle, signatureImage).
\t\tScan(&s.UserID, &s.FullName, &s.FontStyle, &s.SignatureImage)
\tif err != nil {
\t\treturn nil, fmt.Errorf("save user signature: %w", err)
\t}
\treturn &s, nil
}
'''

import os
if os.path.exists(SIGNATURE_REPO_PATH):
    print(f"ERROR: {SIGNATURE_REPO_PATH} already exists. Not overwriting. Aborting.")
    sys.exit(1)

with open(SIGNATURE_REPO_PATH, "w", encoding="utf-8") as f:
    f.write(signature_repo_content)
print(f"Created {SIGNATURE_REPO_PATH}")

# ── 2. Edit internal/handler/esign.go ──────────────────────────────────────
HANDLER_PATH = "internal/handler/esign.go"
with open(HANDLER_PATH, "r", encoding="utf-8") as f:
    handler_src = f.read()
handler_original = handler_src

# 2a. Replace struct + constructor (regex, whitespace-tolerant)
struct_pattern = re.compile(
    r"type ESignHandler struct \{\s*"
    r"repo\s+\*repository\.ESignRepo\s*"
    r"audit\s+\*repository\.AuditRepo\s*"
    r"\}\s*"
    r"func NewESignHandler\(repo \*repository\.ESignRepo, audit \*repository\.AuditRepo\) \*ESignHandler \{\s*"
    r"return &ESignHandler\{repo: repo, audit: audit\}\s*"
    r"\}",
    re.MULTILINE,
)

new_struct = (
    "type ESignHandler struct {\n"
    "\trepo    *repository.ESignRepo\n"
    "\tsigRepo *repository.SignatureRepo\n"
    "\taudit   *repository.AuditRepo\n"
    "}\n\n"
    "func NewESignHandler(repo *repository.ESignRepo, sigRepo *repository.SignatureRepo, audit *repository.AuditRepo) *ESignHandler {\n"
    "\treturn &ESignHandler{repo: repo, sigRepo: sigRepo, audit: audit}\n"
    "}"
)

handler_src, n = struct_pattern.subn(new_struct, handler_src, count=1)
if n != 1:
    print("ERROR: could not find/replace ESignHandler struct + constructor in esign.go (pattern mismatch). Aborting — no files changed beyond signature.go.")
    print("You may need to make this one edit manually; here is what should replace the struct+constructor block:")
    print(new_struct)
    sys.exit(1)

# 2b. Insert two new handler methods before the webhook doc comment
webhook_anchor_pattern = re.compile(r"// POST /webhooks/ilovepdf")
if len(webhook_anchor_pattern.findall(handler_src)) != 1:
    print("ERROR: expected exactly 1 occurrence of '// POST /webhooks/ilovepdf' comment. Aborting.")
    sys.exit(1)

new_methods = '''// GET /api/esign/my-signature
// Returns the caller's saved signature, or data: null if not set up yet.
func (h *ESignHandler) GetMySignature(w http.ResponseWriter, r *http.Request) {
\tuser := auth.GetUser(r.Context())
\tsig, err := h.sigRepo.GetMine(r.Context(), user.ID)
\tif err != nil {
\t\twriteError(w, http.StatusInternalServerError, "failed to load signature")
\t\treturn
\t}
\twriteJSON(w, http.StatusOK, model.Response{Data: sig})
}

// POST /api/esign/my-signature
// Body: { full_name, font_style, signature_image }
// Creates or overwrites the caller's saved signature.
func (h *ESignHandler) SaveMySignature(w http.ResponseWriter, r *http.Request) {
\tvar req struct {
\t\tFullName       string `json:"full_name"`
\t\tFontStyle      string `json:"font_style"`
\t\tSignatureImage string `json:"signature_image"`
\t}
\tif err := decode(r, &req); err != nil {
\t\twriteError(w, http.StatusBadRequest, "invalid request body")
\t\treturn
\t}
\tif req.FullName == "" || req.FontStyle == "" || req.SignatureImage == "" {
\t\twriteError(w, http.StatusBadRequest, "full_name, font_style, and signature_image are required")
\t\treturn
\t}

\tuser := auth.GetUser(r.Context())
\tsig, err := h.sigRepo.SaveMine(r.Context(), user.ID, req.FullName, req.FontStyle, req.SignatureImage)
\tif err != nil {
\t\tlog.Printf("save my-signature failed: %v", err)
\t\twriteError(w, http.StatusInternalServerError, "failed to save signature")
\t\treturn
\t}

\t_ = h.audit.Record(r.Context(), user.ID, "esign_signature_saved", "E-Signatures",
\t\t"Set up personal signature", r.RemoteAddr)
\twriteJSON(w, http.StatusOK, model.Response{Data: sig})
}

'''

handler_src = webhook_anchor_pattern.sub(new_methods + "// POST /webhooks/ilovepdf", handler_src, count=1)

if handler_src == handler_original:
    print("ERROR: no changes made to esign.go (unexpected). Aborting.")
    sys.exit(1)

with open(HANDLER_PATH, "w", encoding="utf-8") as f:
    f.write(handler_src)
print(f"Updated {HANDLER_PATH}")

# ── 3. Edit internal/router/router.go ──────────────────────────────────────
ROUTER_PATH = "internal/router/router.go"
with open(ROUTER_PATH, "r", encoding="utf-8") as f:
    router_src = f.read()
router_original = router_src

create_route_pattern = re.compile(
    r'(r\.Post\("/documents",\s*h\.ESign\.Create\)\s*\n)'
)
matches = create_route_pattern.findall(router_src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of the esign Create route line, found {len(matches)}. Aborting.")
    sys.exit(1)

router_src = create_route_pattern.sub(
    r'\1\t\tr.Get("/my-signature", h.ESign.GetMySignature)\n\t\tr.Post("/my-signature", h.ESign.SaveMySignature)\n',
    router_src,
    count=1,
)

if router_src == router_original:
    print("ERROR: no changes made to router.go (unexpected). Aborting.")
    sys.exit(1)

with open(ROUTER_PATH, "w", encoding="utf-8") as f:
    f.write(router_src)
print(f"Updated {ROUTER_PATH}")

print("\nAll 3 files done.")
print("IMPORTANT: main.go still needs a manual one-line fix wherever NewESignHandler(...) is")
print("called, since its signature now takes an extra sigRepo argument.")
print('Run: grep -n "NewESignHandler\\|NewESignRepo" cmd/server/main.go')
print("and share the output before running go build.")
