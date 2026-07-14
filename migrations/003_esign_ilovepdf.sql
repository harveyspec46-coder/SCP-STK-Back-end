-- ════════════════════════════════════════════════════════════════════════════
-- 003 — Wire E-Signatures to the real iLovePDF Signature REST API.
--
-- Previously esign_documents/esign_signers only supported an in-app canvas
-- signature (base64 PNG in `signature_data`). This migration adds the
-- columns needed to track a real iLovePDF signature request end-to-end:
-- the source PDF, the iLovePDF task/server pair, the requester + signer
-- tokens returned by POST /v1/signature, the raw iLovePDF status (richer
-- than our own pending/complete), and the final signed PDF once it comes
-- back through the webhook.
--
-- `signed`, `signature_data`, and `signed_at` on esign_signers are left in
-- place for backward compatibility with any already-completed mock records,
-- but are no longer written to by the new flow except `signed`/`signed_at`,
-- which are now driven by the `signature.signer.completed` webhook instead
-- of a canvas draw.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE esign_documents
    ADD COLUMN source_file_url   TEXT,             -- PDF uploaded to Supabase Storage by the frontend before creating the request
    ADD COLUMN signed_file_url   TEXT,              -- Final signed PDF, downloaded from iLovePDF and re-stored in Supabase Storage
    ADD COLUMN ilovepdf_server   TEXT,               -- {server} from POST /v1/start/sign — required for every subsequent call on this task
    ADD COLUMN ilovepdf_task     TEXT,                -- task ID from POST /v1/start/sign
    ADD COLUMN token_requester   TEXT,              -- requester token from POST /v1/signature — used for status checks + download-signed
    ADD COLUMN signature_uuid    TEXT,               -- public uuid from POST /v1/signature
    ADD COLUMN ilovepdf_status   TEXT NOT NULL DEFAULT 'draft', -- draft | sent | completed | declined | expired | void | deleted (mirrors iLovePDF's own status field)
    ADD COLUMN completed_at      TIMESTAMPTZ;         -- set when the signature.completed webhook fires

CREATE UNIQUE INDEX idx_esign_documents_token_requester ON esign_documents (token_requester)
    WHERE token_requester IS NOT NULL;

ALTER TABLE esign_signers
    ADD COLUMN email                     TEXT NOT NULL DEFAULT '',  -- required by iLovePDF for every signer
    ADD COLUMN ilovepdf_token_requester   TEXT,                       -- per-signer token from POST /v1/signature response — identifies this signer in webhook payloads
    ADD COLUMN ilovepdf_status            TEXT NOT NULL DEFAULT 'waiting'; -- waiting | sent | viewed | signed | declined | error (mirrors iLovePDF's receiver status)

CREATE UNIQUE INDEX idx_esign_signers_token_requester ON esign_signers (ilovepdf_token_requester)
    WHERE ilovepdf_token_requester IS NOT NULL;

-- Webhook events arrive with no Supabase JWT (iLovePDF calls this endpoint
-- directly), so RLS is irrelevant there — the Go backend's service-role
-- connection updates these rows directly, same as audit_log inserts.
