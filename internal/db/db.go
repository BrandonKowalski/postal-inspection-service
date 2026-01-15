package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS blocked_senders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		reason TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS transactional_only_senders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		reason TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS email_details (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id TEXT,
		sender TEXT,
		recipients TEXT,
		subject TEXT,
		date TEXT,
		headers TEXT,
		body_text TEXT,
		body_html TEXT,
		has_attachments INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS action_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		action TEXT NOT NULL,
		sender TEXT NOT NULL,
		subject TEXT,
		message_id TEXT,
		details TEXT,
		email_detail_id INTEGER,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (email_detail_id) REFERENCES email_details(id)
	);

	CREATE INDEX IF NOT EXISTS idx_blocked_senders_email ON blocked_senders(email);
	CREATE INDEX IF NOT EXISTS idx_transactional_only_senders_email ON transactional_only_senders(email);
	CREATE INDEX IF NOT EXISTS idx_action_log_created_at ON action_log(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_email_details_message_id ON email_details(message_id);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) Close() error {
	return db.conn.Close()
}

// BlockedSender operations

func (db *DB) AddBlockedSender(email, reason string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO blocked_senders (email, reason, created_at) VALUES (?, ?, ?)",
		email, reason, time.Now(),
	)
	return err
}

func (db *DB) RemoveBlockedSender(id int64) error {
	_, err := db.conn.Exec("DELETE FROM blocked_senders WHERE id = ?", id)
	return err
}

func (db *DB) IsBlocked(email string) (bool, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM blocked_senders WHERE email = ?", email).Scan(&count)
	return count > 0, err
}

func (db *DB) GetBlockedSenders() ([]BlockedSender, error) {
	rows, err := db.conn.Query("SELECT id, email, reason, created_at FROM blocked_senders ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var senders []BlockedSender
	for rows.Next() {
		var s BlockedSender
		if err := rows.Scan(&s.ID, &s.Email, &s.Reason, &s.CreatedAt); err != nil {
			return nil, err
		}
		senders = append(senders, s)
	}
	return senders, rows.Err()
}

func (db *DB) GetBlockedSenderByID(id int64) (*BlockedSender, error) {
	var s BlockedSender
	err := db.conn.QueryRow(
		"SELECT id, email, reason, created_at FROM blocked_senders WHERE id = ?", id,
	).Scan(&s.ID, &s.Email, &s.Reason, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// TransactionalOnlySender operations

func (db *DB) AddTransactionalOnlySender(email, reason string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO transactional_only_senders (email, reason, created_at) VALUES (?, ?, ?)",
		email, reason, time.Now(),
	)
	return err
}

func (db *DB) RemoveTransactionalOnlySender(id int64) error {
	_, err := db.conn.Exec("DELETE FROM transactional_only_senders WHERE id = ?", id)
	return err
}

func (db *DB) IsTransactionalOnly(email string) (bool, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM transactional_only_senders WHERE email = ?", email).Scan(&count)
	return count > 0, err
}

func (db *DB) GetTransactionalOnlySenders() ([]TransactionalOnlySender, error) {
	rows, err := db.conn.Query("SELECT id, email, reason, created_at FROM transactional_only_senders ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var senders []TransactionalOnlySender
	for rows.Next() {
		var s TransactionalOnlySender
		if err := rows.Scan(&s.ID, &s.Email, &s.Reason, &s.CreatedAt); err != nil {
			return nil, err
		}
		senders = append(senders, s)
	}
	return senders, rows.Err()
}

func (db *DB) GetTransactionalOnlySenderByID(id int64) (*TransactionalOnlySender, error) {
	var s TransactionalOnlySender
	err := db.conn.QueryRow(
		"SELECT id, email, reason, created_at FROM transactional_only_senders WHERE id = ?", id,
	).Scan(&s.ID, &s.Email, &s.Reason, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// EmailDetail operations

func (db *DB) SaveEmailDetail(detail *EmailDetail) (int64, error) {
	result, err := db.conn.Exec(
		`INSERT INTO email_details (message_id, sender, recipients, subject, date, headers, body_text, body_html, has_attachments, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		detail.MessageID, detail.Sender, detail.Recipients, detail.Subject, detail.Date,
		detail.Headers, detail.BodyText, detail.BodyHTML, detail.HasAttachments, time.Now(),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) GetEmailDetail(id int64) (*EmailDetail, error) {
	var detail EmailDetail
	var hasAttachments int
	err := db.conn.QueryRow(
		`SELECT id, message_id, sender, recipients, subject, date, headers, body_text, body_html, has_attachments, created_at
		 FROM email_details WHERE id = ?`, id,
	).Scan(&detail.ID, &detail.MessageID, &detail.Sender, &detail.Recipients, &detail.Subject, &detail.Date,
		&detail.Headers, &detail.BodyText, &detail.BodyHTML, &hasAttachments, &detail.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	detail.HasAttachments = hasAttachments == 1
	return &detail, nil
}

// ActionLog operations

func (db *DB) LogAction(action, sender, subject, messageID, details string) error {
	_, err := db.conn.Exec(
		"INSERT INTO action_log (action, sender, subject, message_id, details, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		action, sender, subject, messageID, details, time.Now(),
	)
	return err
}

func (db *DB) LogActionWithEmail(action, sender, subject, messageID, details string, emailDetailID int64) error {
	_, err := db.conn.Exec(
		"INSERT INTO action_log (action, sender, subject, message_id, details, email_detail_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		action, sender, subject, messageID, details, emailDetailID, time.Now(),
	)
	return err
}

func (db *DB) GetActionLogs(limit, offset int) ([]ActionLog, error) {
	rows, err := db.conn.Query(
		"SELECT id, action, sender, subject, message_id, details, email_detail_id, created_at FROM action_log ORDER BY created_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ActionLog
	for rows.Next() {
		var l ActionLog
		var subject, messageID, details sql.NullString
		var emailDetailID sql.NullInt64
		if err := rows.Scan(&l.ID, &l.Action, &l.Sender, &subject, &messageID, &details, &emailDetailID, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Subject = subject.String
		l.MessageID = messageID.String
		l.Details = details.String
		if emailDetailID.Valid {
			l.EmailDetailID = &emailDetailID.Int64
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (db *DB) GetActionLogByID(id int64) (*ActionLog, error) {
	var l ActionLog
	var subject, messageID, details sql.NullString
	var emailDetailID sql.NullInt64
	err := db.conn.QueryRow(
		"SELECT id, action, sender, subject, message_id, details, email_detail_id, created_at FROM action_log WHERE id = ?", id,
	).Scan(&l.ID, &l.Action, &l.Sender, &subject, &messageID, &details, &emailDetailID, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.Subject = subject.String
	l.MessageID = messageID.String
	l.Details = details.String
	if emailDetailID.Valid {
		l.EmailDetailID = &emailDetailID.Int64
	}
	return &l, nil
}

func (db *DB) GetActionLogCount() (int, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM action_log").Scan(&count)
	return count, err
}

// PurgeOldEmailDetails deletes email details older than the specified number of days
// and removes references from action_log entries
func (db *DB) PurgeOldEmailDetails(olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)

	// First, clear references in action_log
	_, err := db.conn.Exec(
		`UPDATE action_log SET email_detail_id = NULL
		 WHERE email_detail_id IN (SELECT id FROM email_details WHERE created_at < ?)`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to clear email references: %w", err)
	}

	// Then delete old email details
	result, err := db.conn.Exec(
		"DELETE FROM email_details WHERE created_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old email details: %w", err)
	}

	return result.RowsAffected()
}

// Stats

type Stats struct {
	BlockedSendersCount           int
	TransactionalOnlySendersCount int
	TotalActionsCount             int
	RecentActions                 []ActionLog
}

func (db *DB) GetStats() (*Stats, error) {
	stats := &Stats{}

	if err := db.conn.QueryRow("SELECT COUNT(*) FROM blocked_senders").Scan(&stats.BlockedSendersCount); err != nil {
		return nil, err
	}

	if err := db.conn.QueryRow("SELECT COUNT(*) FROM transactional_only_senders").Scan(&stats.TransactionalOnlySendersCount); err != nil {
		return nil, err
	}

	if err := db.conn.QueryRow("SELECT COUNT(*) FROM action_log").Scan(&stats.TotalActionsCount); err != nil {
		return nil, err
	}

	logs, err := db.GetActionLogs(10, 0)
	if err != nil {
		return nil, err
	}
	stats.RecentActions = logs

	return stats, nil
}
