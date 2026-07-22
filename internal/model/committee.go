package model

import "time"

// ════════════════════════════════════════════════════════════════════════════
// Committees — board-only groups (grants committee, AYP, etc). Admin creates
// them and assigns members with a role (lead/co-lead/member). Docs and
// in-committee messaging land in later stages.
// ════════════════════════════════════════════════════════════════════════════

type Committee struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`

	Members []CommitteeMember `json:"members,omitempty"`
}

type CommitteeMember struct {
	ID          string    `json:"id"`
	CommitteeID string    `json:"committee_id"`
	UserID      string    `json:"user_id"`
	FullName    string    `json:"full_name,omitempty"` // joined from users table for display
	Role        string    `json:"role"`                // lead | co-lead | member
	AddedAt     time.Time `json:"added_at"`
}

type CreateCommitteeRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	MemberIDs   []string `json:"member_ids,omitempty"` // initial members, all added as "member" role
}

type AddCommitteeMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role,omitempty"` // defaults to "member"
}

type UpdateCommitteeMemberRoleRequest struct {
	Role string `json:"role"`
}
