package model

import "time"

// ════════════════════════════════════════════════════════════════════════════
// Committee messages — a simple group chat scoped to one committee's
// members. Real-time delivery is handled client-side via Supabase Realtime
// on the committee_messages table, not by this backend.
// ════════════════════════════════════════════════════════════════════════════

type CommitteeMessage struct {
	ID          string    `json:"id"`
	CommitteeID string    `json:"committee_id"`
	UserID      string    `json:"user_id"`
	FullName    string    `json:"full_name,omitempty"` // joined from users table for display
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

type SendCommitteeMessageRequest struct {
	Body string `json:"body"`
}
