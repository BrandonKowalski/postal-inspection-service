package db

import "time"

type BlockedSender struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type TransactionalOnlySender struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type ActionLog struct {
	ID            int64     `json:"id"`
	Action        string    `json:"action"`
	Sender        string    `json:"sender"`
	Subject       string    `json:"subject"`
	MessageID     string    `json:"message_id"`
	Details       string    `json:"details"`
	EmailDetailID *int64    `json:"email_detail_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type EmailDetail struct {
	ID             int64     `json:"id"`
	MessageID      string    `json:"message_id"`
	Sender         string    `json:"sender"`
	Recipients     string    `json:"recipients"`
	Subject        string    `json:"subject"`
	Date           string    `json:"date"`
	Headers        string    `json:"headers"`
	BodyText       string    `json:"body_text"`
	BodyHTML       string    `json:"body_html"`
	HasAttachments bool      `json:"has_attachments"`
	CreatedAt      time.Time `json:"created_at"`
}

const (
	ActionBlockedSender            = "blocked_sender"
	ActionDeletedEmail             = "deleted_email"
	ActionUnblockedSender          = "unblocked_sender"
	ActionTransactionalOnlySender  = "transactional_only_sender"
	ActionRemovedTransactionalOnly = "removed_transactional_only"
	ActionDeletedMarketing         = "deleted_marketing"
)
