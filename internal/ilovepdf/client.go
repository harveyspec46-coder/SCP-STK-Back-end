// Package ilovepdf is a minimal client for the iLovePDF Signature REST API
// (api.ilovepdf.com), covering exactly the flow SCP-STK Hub needs:
//
//	self-signed JWT  ->  start/sign  ->  upload  ->  signature  ->  download-signed
//
// Auth: iLovePDF supports either self-signing a JWT with the account's
// secret key, or requesting a signed token from their /auth server. We
// always self-sign (recommended for server-side code, and it saves a round
// trip). The exact claim shape below — jti = public key, iss =
// "api.ilovepdf.com", iat = now minus a few seconds, no explicit exp — is
// copied from iLovePDF's own official JS client (ilovepdf-js-core/src/auth/JWT.ts),
// since the public API docs describe the auth *method* but not the exact
// payload shape. The server enforces the 1-hour expiry itself based on iat;
// it does not require an exp claim in the token.
package ilovepdf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	baseURL   = "https://api.ilovepdf.com/v1"
	issuer    = "api.ilovepdf.com"
	tokenSkew = 5 * time.Second // matches the official client's TIME_DELAY
)

// Client talks to the iLovePDF Signature API using self-signed JWT auth.
type Client struct {
	PublicKey string
	SecretKey string
	HTTP      *http.Client
}

func NewClient(publicKey, secretKey string) *Client {
	return &Client{
		PublicKey: publicKey,
		SecretKey: secretKey,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
}

// signToken self-signs a fresh JWT for one request. iLovePDF tokens are
// short-lived (1hr, server-enforced), so we mint a new one per call rather
// than caching — signature requests happen rarely enough in this app that
// the extra HMAC-SHA256 sign is not worth the complexity of a cache.
func (c *Client) signToken() (string, error) {
	claims := jwt.MapClaims{
		"jti": c.PublicKey,
		"iss": issuer,
		"iat": time.Now().Add(-tokenSkew).UTC().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(c.SecretKey))
	if err != nil {
		return "", fmt.Errorf("self-sign ilovepdf jwt: %w", err)
	}
	return signed, nil
}

func (c *Client) authHeader() (string, error) {
	tok, err := c.signToken()
	if err != nil {
		return "", err
	}
	return "Bearer " + tok, nil
}

// ─── Start ──────────────────────────────────────────────────────────────────

type startSignResponse struct {
	Server           string `json:"server"`
	Task             string `json:"task"`
	RemainingCredits int    `json:"remaining_credits"`
}

// StartSign calls POST /v1/start/sign and returns the assigned server and
// task ID that every subsequent call for this signature request must use.
func (c *Client) StartSign(ctx context.Context) (server, task string, err error) {
	auth, err := c.authHeader()
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/start/sign", nil)
	if err != nil {
		return "", "", fmt.Errorf("build start/sign request: %w", err)
	}
	req.Header.Set("Authorization", auth)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("call start/sign: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("start/sign returned %d: %s", resp.StatusCode, string(body))
	}

	var out startSignResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", fmt.Errorf("decode start/sign response: %w", err)
	}
	return out.Server, out.Task, nil
}

// ─── Upload ─────────────────────────────────────────────────────────────────

type uploadResponse struct {
	ServerFilename string `json:"server_filename"`
}

// Upload sends a PDF's raw bytes to the given task on its assigned server
// and returns the server_filename to reference it in later calls.
func (c *Client) Upload(ctx context.Context, server, task, filename string, fileBytes []byte) (serverFilename string, err error) {
	auth, err := c.authHeader()
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("task", task); err != nil {
		return "", fmt.Errorf("write task field: %w", err)
	}
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		return "", fmt.Errorf("write file bytes: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	url := fmt.Sprintf("https://%s/v1/upload", server)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("call upload: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload returned %d: %s", resp.StatusCode, string(respBody))
	}

	var out uploadResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}
	return out.ServerFilename, nil
}

// ─── Create signature request ──────────────────────────────────────────────

// Element is a single field placed on a page for a signer (signature, date,
// name, text, or initials). Position uses iLovePDF's "gravity" format, e.g.
// "bottom center" or "top right" — see the API reference for the full list.
type Element struct {
	Type     string `json:"type"`              // signature | initials | name | date | text | input
	Page     string `json:"pages"`             // "1", "3-12", "-1" (last page), etc.
	Position string `json:"position"`          // e.g. "bottom center"
	Content  string `json:"content,omitempty"` // required for type=date (date format) or type=text (the text)
	Size     int    `json:"size,omitempty"`
}

// SignerFile pairs an uploaded file with the field elements one signer must
// complete on it.
type SignerFile struct {
	ServerFilename string    `json:"server_filename"`
	Elements       []Element `json:"elements"`
}

// Signer is one receiver of the signature request.
type Signer struct {
	Name  string       `json:"name"`
	Email string       `json:"email"`
	Type  string       `json:"type"` // signer | validator | viewer
	Files []SignerFile `json:"files"`
}

// FileEntry is one file attached to the overall signature request.
type FileEntry struct {
	ServerFilename string `json:"server_filename"`
	Filename       string `json:"filename"`
}

type createSignatureRequest struct {
	Task            string      `json:"task"`
	Files           []FileEntry `json:"files"`
	Signers         []Signer    `json:"signers"`
	UUIDVisible     bool        `json:"uuid_visible"`
	VerifyEnabled   bool        `json:"verify_enabled"`
	SignerReminders bool        `json:"signer_reminders"`
}

// SignatureResponse is the subset of iLovePDF's create-signature /
// get-signature-status response we care about.
type SignatureResponse struct {
	TokenRequester string         `json:"token_requester"`
	UUID           string         `json:"uuid"`
	Status         string         `json:"status"`
	Signers        []SignerStatus `json:"signers"`
}

type SignerStatus struct {
	UUID           string `json:"uuid"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	TokenRequester string `json:"token_requester"`
	Status         string `json:"status"`
}

// CreateSignature calls POST /v1/signature to create the signature request,
// which emails every signer and returns the requester + per-signer tokens
// needed to track status via webhook and later download the signed PDF.
//
// uuid_visible is always sent as true per iLovePDF's own recommendation —
// disabling it lowers the legal validity of the resulting signatures, which
// runs counter to the whole reason this integration exists (eIDAS/ESIGN/UETA
// compliance).
func (c *Client) CreateSignature(ctx context.Context, server, task string, files []FileEntry, signers []Signer) (*SignatureResponse, error) {
	auth, err := c.authHeader()
	if err != nil {
		return nil, err
	}

	payload := createSignatureRequest{
		Task:            task,
		Files:           files,
		Signers:         signers,
		UUIDVisible:     true,
		VerifyEnabled:   true,
		SignerReminders: true,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal create-signature payload: %w", err)
	}

	url := fmt.Sprintf("https://%s/v1/signature", server)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build create-signature request: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call create-signature: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create-signature returned %d: %s", resp.StatusCode, string(respBody))
	}

	var out SignatureResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode create-signature response: %w", err)
	}
	return &out, nil
}

// ─── Webhooks ───────────────────────────────────────────────────────────────
//
// iLovePDF POSTs these to the endpoint registered in the iloveapi.com
// dashboard (Webhooks section) — there is no way to register it via the API
// itself. Every payload has the same {event, data} envelope; signature
// events additionally nest a "signer" object when the event concerns one
// specific receiver (e.g. signature.signer.completed).

type WebhookPayload struct {
	Event string      `json:"event"`
	Data  WebhookData `json:"data"`
}

type WebhookData struct {
	Signature *SignatureResponse `json:"signature,omitempty"`
	Signer    *SignerStatus      `json:"signer,omitempty"`
}

// Known event names this app acts on. iLovePDF sends others (task.completed,
// task.failed, signature.created, signature.declined, ...) — anything not
// explicitly handled by the webhook handler is safely ignored.
const (
	EventSignerCompleted    = "signature.signer.completed"
	EventSignatureCompleted = "signature.completed"
	EventSignatureDeclined  = "signature.declined"
	EventSignatureExpired   = "signature.expired"
	EventSignatureVoided    = "signature.void"
)

// ─── Download signed ────────────────────────────────────────────────────────

// DownloadSigned fetches the completed, signed PDF for a signature request.
// Only valid once the request's status is "completed" — otherwise iLovePDF
// returns 400.
func (c *Client) DownloadSigned(ctx context.Context, server, tokenRequester string) ([]byte, error) {
	auth, err := c.authHeader()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://%s/v1/signature/%s/download-signed", server, tokenRequester)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build download-signed request: %w", err)
	}
	req.Header.Set("Authorization", auth)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call download-signed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read download-signed body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download-signed returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
