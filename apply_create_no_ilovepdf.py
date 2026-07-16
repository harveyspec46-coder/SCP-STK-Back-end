#!/usr/bin/env python3
"""
Replaces ESignRepo.Create with a version that saves the document + signers
directly to the database, with no calls to the iLovePDF Signature API
(that flow was abandoned when the project moved to fully in-app signing).
Run from ~/scp-hub-go. Usage: python3 apply_create_no_ilovepdf.py
"""
import sys

PATH = "internal/repository/esign.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

old_create = '''// Create fetches the already-uploaded source PDF, runs the full iLovePDF
// start -> upload -> create-signature flow (which emails every signer),
// then persists the document + signer rows with their iLovePDF tokens.
//
// All external calls happen before any DB write, so a failure partway
// through (e.g. iLovePDF create-signature rejects a bad email) never leaves
// a half-created document behind — the orphaned iLovePDF task simply expires
// on their side after 2 hours per their own task-lifetime policy.
func (r *ESignRepo) Create(ctx context.Context, req model.CreateESignDocumentRequest, createdBy string) (*model.ESignDocument, error) {
\tif req.SourceFileURL == "" {
\t\treturn nil, fmt.Errorf("source_file_url is required")
\t}
\tif len(req.Signers) == 0 {
\t\treturn nil, fmt.Errorf("at least one signer is required")
\t}

\tpdfBytes, err := storage.FetchFile(ctx, r.ilovepdf.HTTP, req.SourceFileURL)
\tif err != nil {
\t\treturn nil, fmt.Errorf("fetch source pdf: %w", err)
\t}
\tfilename := filenameFromURL(req.SourceFileURL)

\tserver, task, err := r.ilovepdf.StartSign(ctx)
\tif err != nil {
\t\treturn nil, fmt.Errorf("start ilovepdf sign task: %w", err)
\t}

\tserverFilename, err := r.ilovepdf.Upload(ctx, server, task, filename, pdfBytes)
\tif err != nil {
\t\treturn nil, fmt.Errorf("upload pdf to ilovepdf: %w", err)
\t}

\tilpSigners := make([]ilovepdf.Signer, 0, len(req.Signers))
\tfor _, s := range req.Signers {
\t\telements := s.Elements
\t\tif len(elements) == 0 {
\t\t\telements = defaultElements()
\t\t}
\t\tilpSigners = append(ilpSigners, ilovepdf.Signer{
\t\t\tName:  s.Name,
\t\t\tEmail: s.Email,
\t\t\tType:  "signer",
\t\t\tFiles: []ilovepdf.SignerFile{
\t\t\t\t{ServerFilename: serverFilename, Elements: toILovePDFElements(elements)},
\t\t\t},
\t\t})
\t}

\tsigResp, err := r.ilovepdf.CreateSignature(ctx, server, task,
\t\t[]ilovepdf.FileEntry{{ServerFilename: serverFilename, Filename: filename}}, ilpSigners)
\tif err != nil {
\t\treturn nil, fmt.Errorf("create ilovepdf signature request: %w", err)
\t}

\t// Match returned per-signer tokens back to our request by email, since
\t// iLovePDF's response order isn't documented as guaranteed to match ours.
\ttokenByEmail := make(map[string]string, len(sigResp.Signers))
\tstatusByEmail := make(map[string]string, len(sigResp.Signers))
\tfor _, s := range sigResp.Signers {
\t\ttokenByEmail[strings.ToLower(s.Email)] = s.TokenRequester
\t\tstatusByEmail[strings.ToLower(s.Email)] = s.Status
\t}

\ttx, err := r.db.Begin(ctx)
\tif err != nil {
\t\treturn nil, fmt.Errorf("begin tx: %w", err)
\t}
\tdefer tx.Rollback(ctx)

\tid := uuid.New().String()
\tvar d model.ESignDocument
\terr = tx.QueryRow(ctx,
\t\t`INSERT INTO esign_documents
\t\t   (id, name, type, pages, clauses, status, created_by, source_file_url,
\t\t    ilovepdf_server, ilovepdf_task, token_requester, signature_uuid, ilovepdf_status)
\t\t VALUES ($1,$2,$3,1,$4,'pending',$5,$6,$7,$8,$9,$10,$11)
\t\t RETURNING id, name, type, pages, clauses, status, created_by, created_at,
\t\t           source_file_url, signed_file_url, ilovepdf_server, ilovepdf_task,
\t\t           token_requester, signature_uuid, ilovepdf_status, completed_at`,
\t\tid, req.Name, req.Type, req.Clauses, createdBy, req.SourceFileURL,
\t\tserver, task, sigResp.TokenRequester, sigResp.UUID, sigResp.Status).
\t\tScan(&d.ID, &d.Name, &d.Type, &d.Pages, &d.Clauses, &d.Status, &d.CreatedBy, &d.CreatedAt,
\t\t\t&d.SourceFileURL, &d.SignedFileURL, &d.ILovePDFServer, &d.ILovePDFTask,
\t\t\t&d.TokenRequester, &d.SignatureUUID, &d.ILovePDFStatus, &d.CompletedAt)
\tif err != nil {
\t\treturn nil, fmt.Errorf("insert esign document: %w", err)
\t}

\tfor _, s := range req.Signers {
\t\trole := s.Role
\t\tif role == "" {
\t\t\trole = "external"
\t\t}
\t\tsid := uuid.New().String()
\t\ttoken := tokenByEmail[strings.ToLower(s.Email)]
\t\tstatus := statusByEmail[strings.ToLower(s.Email)]
\t\tif status == "" {
\t\t\tstatus = "waiting"
\t\t}
\t\tif _, err := tx.Exec(ctx,
\t\t\t`INSERT INTO esign_signers (id, document_id, name, email, role, signed, ilovepdf_status, ilovepdf_token_requester)
\t\t\t VALUES ($1,$2,$3,$4,$5,false,$6,$7)`,
\t\t\tsid, id, s.Name, s.Email, role, status, token); err != nil {
\t\t\treturn nil, fmt.Errorf("create signer %q: %w", s.Email, err)
\t\t}
\t}

\tif err := tx.Commit(ctx); err != nil {
\t\treturn nil, fmt.Errorf("commit tx: %w", err)
\t}

\td.Signers, _ = r.signersForDoc(ctx, id)
\treturn &d, nil
}'''

count = src.count(old_create)
if count != 1:
    print(f"ERROR: expected exactly 1 occurrence of the old Create() method, found {count}. Aborting — file left unchanged.")
    print("This likely means the file has already been modified since the version this script expects.")
    sys.exit(1)

new_create = '''// Create saves the document + signers directly to the database. Signing
// happens entirely in-app (drag a saved signature onto a pre-placed field —
// see esign_fields.go), so there is no external iLovePDF call here; this
// method used to run the full iLovePDF start/sign -> upload -> create-
// signature flow, which was dropped when the project moved off it.
func (r *ESignRepo) Create(ctx context.Context, req model.CreateESignDocumentRequest, createdBy string) (*model.ESignDocument, error) {
\tif req.SourceFileURL == "" {
\t\treturn nil, fmt.Errorf("source_file_url is required")
\t}
\tif len(req.Signers) == 0 {
\t\treturn nil, fmt.Errorf("at least one signer is required")
\t}

\ttx, err := r.db.Begin(ctx)
\tif err != nil {
\t\treturn nil, fmt.Errorf("begin tx: %w", err)
\t}
\tdefer tx.Rollback(ctx)

\tid := uuid.New().String()
\tvar d model.ESignDocument
\terr = tx.QueryRow(ctx,
\t\t`INSERT INTO esign_documents (id, name, type, pages, clauses, status, created_by, source_file_url)
\t\t VALUES ($1,$2,$3,1,$4,'pending',$5,$6)
\t\t RETURNING id, name, type, pages, clauses, status, created_by, created_at,
\t\t           source_file_url, signed_file_url, ilovepdf_server, ilovepdf_task,
\t\t           token_requester, signature_uuid, ilovepdf_status, completed_at`,
\t\tid, req.Name, req.Type, req.Clauses, createdBy, req.SourceFileURL).
\t\tScan(&d.ID, &d.Name, &d.Type, &d.Pages, &d.Clauses, &d.Status, &d.CreatedBy, &d.CreatedAt,
\t\t\t&d.SourceFileURL, &d.SignedFileURL, &d.ILovePDFServer, &d.ILovePDFTask,
\t\t\t&d.TokenRequester, &d.SignatureUUID, &d.ILovePDFStatus, &d.CompletedAt)
\tif err != nil {
\t\treturn nil, fmt.Errorf("insert esign document: %w", err)
\t}

\tfor _, s := range req.Signers {
\t\trole := s.Role
\t\tif role == "" {
\t\t\trole = "external"
\t\t}
\t\tsid := uuid.New().String()
\t\tif _, err := tx.Exec(ctx,
\t\t\t`INSERT INTO esign_signers (id, document_id, name, email, role, signed)
\t\t\t VALUES ($1,$2,$3,$4,$5,false)`,
\t\t\tsid, id, s.Name, s.Email, role); err != nil {
\t\t\treturn nil, fmt.Errorf("create signer %q: %w", s.Email, err)
\t\t}
\t}

\tif err := tx.Commit(ctx); err != nil {
\t\treturn nil, fmt.Errorf("commit tx: %w", err)
\t}

\td.Signers, _ = r.signersForDoc(ctx, id)
\treturn &d, nil
}'''

src = src.replace(old_create, new_create, 1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — Create() rewritten to skip the iLovePDF Signature API entirely.")
print("Next: go build ./...")
print("Note: this may leave 'strings' and defaultElements/toILovePDFElements unused")
print("depending on what else references them — the build step will surface that,")
print("paste back any errors.")
