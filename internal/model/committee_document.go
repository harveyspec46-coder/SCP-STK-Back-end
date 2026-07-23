package model

import "time"

// ════════════════════════════════════════════════════════════════════════════
// Committee documents — materials attached to a committee. Any member can
// upload; the uploader or an admin can delete.
// ════════════════════════════════════════════════════════════════════════════

type CommitteeDocument struct {
	ID          string    `json:"id"`
	CommitteeID string    `json:"committee_id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	FileType    string    `json:"file_type,omitempty"`
	UploadedBy  string    `json:"uploaded_by"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

type AddCommitteeDocumentRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	FileType string `json:"file_type,omitempty"`
}
