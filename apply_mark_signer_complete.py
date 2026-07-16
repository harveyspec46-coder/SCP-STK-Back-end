#!/usr/bin/env python3
"""
Adds MarkSignerComplete to ESignRepo — flips esign_signers.signed once all
of a signer's fields are filled, and flips the document to complete once
every signer is done.
Run from ~/scp-hub-go. Usage: python3 apply_mark_signer_complete.py
"""
import re
import sys

PATH = "internal/repository/esign.go"

with open(PATH, "r", encoding="utf-8") as f:
    src = f.read()
original = src

anchor_pattern = re.compile(r"// UpdateDocumentStatus mirrors")
matches = anchor_pattern.findall(src)
if len(matches) != 1:
    print(f"ERROR: expected exactly 1 occurrence of '// UpdateDocumentStatus mirrors' comment, found {len(matches)}. Aborting.")
    sys.exit(1)

new_method = '''// MarkSignerComplete flips one signer's signed flag once all of their
// esign_fields are filled, then checks whether every signer on the document
// is now done — if so, flips the document itself to complete.
func (r *ESignRepo) MarkSignerComplete(ctx context.Context, documentID, signerID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE esign_signers SET signed = true, signed_at = NOW() WHERE id = $1`,
		signerID); err != nil {
		return fmt.Errorf("mark signer signed: %w", err)
	}

	var remaining int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM esign_signers WHERE document_id = $1 AND signed = false`,
		documentID).Scan(&remaining); err != nil {
		return fmt.Errorf("count remaining signers: %w", err)
	}

	if remaining == 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE esign_documents SET status = 'complete', completed_at = NOW() WHERE id = $1`,
			documentID); err != nil {
			return fmt.Errorf("mark document complete: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// UpdateDocumentStatus mirrors'''

src = anchor_pattern.sub(new_method, src, count=1)

if src == original:
    print("ERROR: no changes made (unexpected). Aborting.")
    sys.exit(1)

with open(PATH, "w", encoding="utf-8") as f:
    f.write(src)

print("Success — MarkSignerComplete added to esign.go.")
