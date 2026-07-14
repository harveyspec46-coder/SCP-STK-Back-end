// Package storage provides a thin server-side client for Supabase Storage.
//
// Every other file upload in this codebase follows a client-uploads-first
// pattern (see TaskRepo.AddAttachment / MOU attachments): the frontend
// pushes the file straight to Supabase Storage and posts the resulting URL
// to the backend. E-Signatures mostly keeps that pattern for the *source*
// PDF (the frontend uploads it, then gives us the URL), but the *signed*
// PDF comes back from iLovePDF as raw bytes inside our Go process, so this
// is the first place the backend itself needs to push a file into Storage
// rather than just recording a URL someone else already produced.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the Supabase Storage REST API using the service role key,
// which bypasses bucket RLS the same way the Go backend's Postgres
// connection bypasses table RLS elsewhere in this app.
type Client struct {
	BaseURL    string // e.g. https://xxxxx.supabase.co
	ServiceKey string
	Bucket     string
	HTTP       *http.Client
}

func NewClient(baseURL, serviceKey, bucket string) *Client {
	return &Client{
		BaseURL:    baseURL,
		ServiceKey: serviceKey,
		Bucket:     bucket,
		HTTP:       &http.Client{Timeout: 60 * time.Second},
	}
}

// Upload pushes data to {bucket}/{path} and returns its public URL.
// The esign-documents bucket must be set to public in the Supabase
// dashboard for the returned URL to be usable by board members' browsers
// without a signed-URL round trip; if it's ever made private instead, this
// method needs to switch to creating a signed URL rather than returning the
// public one.
func (c *Client) Upload(ctx context.Context, path string, data []byte, contentType string) (publicURL string, err error) {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", c.BaseURL, c.Bucket, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build storage upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.ServiceKey)
	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true") // overwrite if a file already exists at this path

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload to supabase storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("supabase storage upload returned %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Sprintf("%s/storage/v1/object/public/%s/%s", c.BaseURL, c.Bucket, path), nil
}

// FetchFile downloads a file from any URL — used to pull the source PDF
// back down from Supabase Storage (or any other URL the frontend hands us)
// before forwarding its bytes to iLovePDF's /upload endpoint.
func FetchFile(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build fetch request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch file returned %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read fetched file body: %w", err)
	}
	return data, nil
}
