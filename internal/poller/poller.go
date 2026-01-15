package poller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"postal-inspection-service/internal/classifier"
	"postal-inspection-service/internal/db"
	"postal-inspection-service/internal/imap"
)

// excludedFolders are folders that should not be scanned for blocked/marketing emails
var excludedFolders = map[string]bool{
	"Orders":                   true,
	"USPIS":                    true,
	"USPIS/Block":              true,
	"USPIS/Transactional Only": true,
	"Sent Messages":            true,
	"Drafts":                   true,
	"Deleted Messages":         true,
}

type Poller struct {
	client   *imap.Client
	db       *db.DB
	interval time.Duration
}

func New(client *imap.Client, database *db.DB, interval time.Duration) *Poller {
	return &Poller{
		client:   client,
		db:       database,
		interval: interval,
	}
}

func (p *Poller) Start(ctx context.Context) {
	log.Printf("Starting poller with interval %v", p.interval)

	// Ensure USPIS folder structure exists
	if err := p.client.CreateUSPISFolders(); err != nil {
		log.Printf("Warning: Could not create USPIS folders: %v", err)
	}

	// Run immediately on start
	p.poll()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Poller stopped")
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

func (p *Poller) poll() {
	log.Println("Polling for emails...")

	// Step 1: Process USPIS/Block folder - add senders to blocked list
	if err := p.processBlockFolder(); err != nil {
		log.Printf("Error processing Block folder: %v", err)
	}

	// Step 2: Process USPIS/Transactional Only folder - add senders to transactional-only list
	if err := p.processTransactionalOnlyFolder(); err != nil {
		log.Printf("Error processing Transactional Only folder: %v", err)
	}

	// Step 3: Delete emails from blocked senders in INBOX
	if err := p.deleteBlockedSenderEmails(); err != nil {
		log.Printf("Error deleting blocked sender emails: %v", err)
	}

	// Step 4: Filter marketing emails from transactional-only senders
	if err := p.filterMarketingEmails(); err != nil {
		log.Printf("Error filtering marketing emails: %v", err)
	}

	log.Println("Poll complete")
}

func (p *Poller) processBlockFolder() error {
	emails, err := p.client.FetchFullEmailsFromBlockFolder()
	if err != nil {
		if strings.Contains(err.Error(), "failed to select folder") {
			log.Println("Block folder not found or empty")
			return nil
		}
		return fmt.Errorf("failed to fetch emails from Block folder: %w", err)
	}

	if len(emails) == 0 {
		return nil
	}

	log.Printf("Found %d emails in USPIS/Block folder", len(emails))

	var uidsToDelete []uint32

	for _, email := range emails {
		senderEmail := strings.ToLower(email.From)
		if senderEmail == "" {
			continue
		}

		// Save email details to database
		emailDetailID, saveErr := p.saveEmailDetail(&email)
		if saveErr != nil {
			log.Printf("Error saving email detail: %v", saveErr)
		}

		blocked, err := p.db.IsBlocked(senderEmail)
		if err != nil {
			log.Printf("Error checking if sender is blocked: %v", err)
			continue
		}

		if !blocked {
			reason := fmt.Sprintf("Moved to Block folder: %s", email.Subject)
			if err := p.db.AddBlockedSender(senderEmail, reason); err != nil {
				log.Printf("Error adding blocked sender: %v", err)
			} else {
				log.Printf("Blocked sender: %s", senderEmail)
				p.logActionWithEmailDetail(
					db.ActionBlockedSender,
					senderEmail,
					email.Subject,
					email.MessageID,
					"Blocked via USPIS/Block folder",
					emailDetailID,
				)
			}
		}

		uidsToDelete = append(uidsToDelete, email.UID)
		p.logActionWithEmailDetail(
			db.ActionDeletedEmail,
			senderEmail,
			email.Subject,
			email.MessageID,
			"Deleted from Block folder",
			emailDetailID,
		)
	}

	if len(uidsToDelete) > 0 {
		if err := p.client.DeleteEmailsFromBlockFolder(uidsToDelete); err != nil {
			return fmt.Errorf("failed to delete emails from Block folder: %w", err)
		}
		log.Printf("Deleted %d emails from Block folder", len(uidsToDelete))
	}

	return nil
}

func (p *Poller) processTransactionalOnlyFolder() error {
	emails, err := p.client.FetchFullEmailsFromTransactionalOnlyFolder()
	if err != nil {
		if strings.Contains(err.Error(), "failed to select folder") {
			log.Println("Transactional Only folder not found or empty")
			return nil
		}
		return fmt.Errorf("failed to fetch emails from Transactional Only folder: %w", err)
	}

	if len(emails) == 0 {
		return nil
	}

	log.Printf("Found %d emails in USPIS/Transactional Only folder", len(emails))

	var uidsToDelete []uint32

	for _, email := range emails {
		senderEmail := strings.ToLower(email.From)
		if senderEmail == "" {
			continue
		}

		// Save email details to database
		emailDetailID, saveErr := p.saveEmailDetail(&email)
		if saveErr != nil {
			log.Printf("Error saving email detail: %v", saveErr)
		}

		isTransactionalOnly, err := p.db.IsTransactionalOnly(senderEmail)
		if err != nil {
			log.Printf("Error checking if sender is transactional-only: %v", err)
			continue
		}

		if !isTransactionalOnly {
			reason := fmt.Sprintf("Moved to Transactional Only folder: %s", email.Subject)
			if err := p.db.AddTransactionalOnlySender(senderEmail, reason); err != nil {
				log.Printf("Error adding transactional-only sender: %v", err)
			} else {
				log.Printf("Added transactional-only sender: %s", senderEmail)
				p.logActionWithEmailDetail(
					db.ActionTransactionalOnlySender,
					senderEmail,
					email.Subject,
					email.MessageID,
					"Added via USPIS/Transactional Only folder - marketing emails will be deleted",
					emailDetailID,
				)
			}
		}

		uidsToDelete = append(uidsToDelete, email.UID)
		p.logActionWithEmailDetail(
			db.ActionDeletedEmail,
			senderEmail,
			email.Subject,
			email.MessageID,
			"Deleted from Transactional Only folder",
			emailDetailID,
		)
	}

	if len(uidsToDelete) > 0 {
		if err := p.client.DeleteEmailsFromTransactionalOnlyFolder(uidsToDelete); err != nil {
			return fmt.Errorf("failed to delete emails from Transactional Only folder: %w", err)
		}
		log.Printf("Deleted %d emails from Transactional Only folder", len(uidsToDelete))
	}

	return nil
}

func (p *Poller) deleteBlockedSenderEmails() error {
	blockedSenders, err := p.db.GetBlockedSenders()
	if err != nil {
		return fmt.Errorf("failed to get blocked senders: %w", err)
	}

	if len(blockedSenders) == 0 {
		log.Println("No blocked senders in database")
		return nil
	}

	senderAddresses := make([]string, len(blockedSenders))
	for i, s := range blockedSenders {
		senderAddresses[i] = s.Email
	}
	log.Printf("Checking %d blocked senders", len(senderAddresses))

	// Get all folders and filter excluded ones
	allFolders, err := p.client.ListFolders()
	if err != nil {
		return fmt.Errorf("failed to list folders: %w", err)
	}

	var folders []string
	for _, folder := range allFolders {
		if !excludedFolders[folder] {
			folders = append(folders, folder)
		}
	}
	log.Printf("Scanning %d folders (excluded %d)", len(folders), len(allFolders)-len(folders))

	// Scan all folders with a single connection
	results, err := p.client.ScanFoldersForSenders(folders, senderAddresses)
	if err != nil {
		return fmt.Errorf("failed to scan folders: %w", err)
	}

	if len(results) == 0 {
		log.Println("No emails found from blocked senders")
		return nil
	}

	// Collect deletions and log actions
	toDelete := make(map[string][]uint32)
	var totalDeleted int

	for _, result := range results {
		log.Printf("Found %d emails from blocked senders in %s", len(result.Emails), result.Folder)

		var uids []uint32
		for _, email := range result.Emails {
			uids = append(uids, email.UID)
			p.db.LogAction(
				db.ActionDeletedEmail,
				email.From,
				email.Subject,
				email.MessageID,
				fmt.Sprintf("Auto-deleted email from blocked sender (folder: %s)", result.Folder),
			)
		}
		toDelete[result.Folder] = uids
		totalDeleted += len(uids)
	}

	// Delete all with a single connection
	if err := p.client.DeleteEmailsFromFolders(toDelete); err != nil {
		return fmt.Errorf("failed to delete emails: %w", err)
	}

	if totalDeleted > 0 {
		log.Printf("Deleted %d total emails from blocked senders across all folders", totalDeleted)
	}
	return nil
}

func (p *Poller) filterMarketingEmails() error {
	transactionalOnlySenders, err := p.db.GetTransactionalOnlySenders()
	if err != nil {
		return fmt.Errorf("failed to get transactional-only senders: %w", err)
	}

	if len(transactionalOnlySenders) == 0 {
		log.Println("No transactional-only senders in database")
		return nil
	}

	senderAddresses := make([]string, len(transactionalOnlySenders))
	for i, s := range transactionalOnlySenders {
		senderAddresses[i] = s.Email
	}
	log.Printf("Checking %d transactional-only senders", len(senderAddresses))

	// Get all folders and filter excluded ones
	allFolders, err := p.client.ListFolders()
	if err != nil {
		return fmt.Errorf("failed to list folders: %w", err)
	}

	var folders []string
	for _, folder := range allFolders {
		if !excludedFolders[folder] {
			folders = append(folders, folder)
		}
	}

	// Scan all folders with a single connection
	results, err := p.client.ScanFoldersForSenders(folders, senderAddresses)
	if err != nil {
		return fmt.Errorf("failed to scan folders: %w", err)
	}

	if len(results) == 0 {
		return nil
	}

	// Process results and collect deletions
	toDelete := make(map[string][]uint32)
	var totalDeleted, totalKept int

	for _, result := range results {
		var uidsToDelete []uint32
		var keptCount int

		for _, email := range result.Emails {
			classification := classifier.Classify(email.Subject)

			if classification.IsTransactional {
				keptCount++
				log.Printf("Keeping transactional email from %s in %s: %s (%s)",
					email.From, result.Folder, email.Subject, classification.Reason)
			} else {
				uidsToDelete = append(uidsToDelete, email.UID)
				p.db.LogAction(
					db.ActionDeletedMarketing,
					email.From,
					email.Subject,
					email.MessageID,
					fmt.Sprintf("Deleted marketing email from folder %s (reason: %s)", result.Folder, classification.Reason),
				)
				log.Printf("Deleting marketing email from %s in %s: %s (%s)",
					email.From, result.Folder, email.Subject, classification.Reason)
			}
		}

		if len(uidsToDelete) > 0 {
			toDelete[result.Folder] = uidsToDelete
		}
		totalDeleted += len(uidsToDelete)
		totalKept += keptCount
	}

	// Delete all with a single connection
	if len(toDelete) > 0 {
		if err := p.client.DeleteEmailsFromFolders(toDelete); err != nil {
			return fmt.Errorf("failed to delete marketing emails: %w", err)
		}
	}

	if totalDeleted > 0 || totalKept > 0 {
		log.Printf("Deleted %d marketing emails, kept %d transactional emails across all folders",
			totalDeleted, totalKept)
	}

	return nil
}

// saveEmailDetail saves email details to the database and returns the ID
func (p *Poller) saveEmailDetail(email *imap.FetchedEmail) (int64, error) {
	detail := &db.EmailDetail{
		MessageID:      email.MessageID,
		Sender:         email.From,
		Recipients:     email.To,
		Subject:        email.Subject,
		Date:           email.Date,
		Headers:        email.Headers,
		BodyText:       email.BodyText,
		BodyHTML:       email.BodyHTML,
		HasAttachments: email.HasAttachments,
	}
	return p.db.SaveEmailDetail(detail)
}

// logActionWithEmailDetail logs an action with optional email detail reference
func (p *Poller) logActionWithEmailDetail(action, sender, subject, messageID, details string, emailDetailID int64) {
	if emailDetailID > 0 {
		if err := p.db.LogActionWithEmail(action, sender, subject, messageID, details, emailDetailID); err != nil {
			log.Printf("Error logging action with email: %v", err)
			// Fall back to regular logging
			p.db.LogAction(action, sender, subject, messageID, details)
		}
	} else {
		p.db.LogAction(action, sender, subject, messageID, details)
	}
}
