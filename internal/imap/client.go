package imap

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Folder paths for USPIS
const (
	FolderBlock             = "USPIS/Block"
	FolderTransactionalOnly = "USPIS/Transactional Only"
)

// Email represents a simplified email message
type Email struct {
	UID       uint32
	MessageID string
	From      string
	Subject   string
	Flags     []string
}

// FetchedEmail represents a full email with body content
type FetchedEmail struct {
	UID            uint32
	MessageID      string
	From           string
	To             string
	Subject        string
	Date           string
	Headers        string
	BodyText       string
	BodyHTML       string
	HasAttachments bool
}

// Client wraps IMAP operations for iCloud
type Client struct {
	server   string
	port     int
	email    string
	password string
}

// NewClient creates a new IMAP client configuration
func NewClient(server string, port int, email, password string) *Client {
	return &Client{
		server:   server,
		port:     port,
		email:    email,
		password: password,
	}
}

// connect establishes a connection to the IMAP server
func (c *Client) connect() (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", c.server, c.port)

	client, err := imapclient.DialTLS(addr, &imapclient.Options{
		TLSConfig: &tls.Config{
			ServerName: c.server,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	if err := client.Login(c.email, c.password).Wait(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	return client, nil
}

// ListFolders returns all folders in the mailbox
func (c *Client) ListFolders() ([]string, error) {
	client, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	listCmd := client.List("", "*", nil)
	var folders []string

	for {
		mbox := listCmd.Next()
		if mbox == nil {
			break
		}
		folders = append(folders, mbox.Mailbox)
	}

	if err := listCmd.Close(); err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	return folders, nil
}

// CreateUSPISFolders ensures the USPIS folder structure exists
func (c *Client) CreateUSPISFolders() error {
	client, err := c.connect()
	if err != nil {
		return err
	}
	defer client.Close()

	folders := []string{"USPIS", FolderBlock, FolderTransactionalOnly}

	for _, folder := range folders {
		// Try to select to check if exists
		_, err = client.Select(folder, nil).Wait()
		if err == nil {
			continue // Folder exists
		}

		// Create the folder
		if err := client.Create(folder, nil).Wait(); err != nil {
			// Ignore error if folder already exists
			if !strings.Contains(err.Error(), "ALREADYEXISTS") {
				log.Printf("Note: Could not create folder %s: %v", folder, err)
			}
		} else {
			log.Printf("Created folder: %s", folder)
		}
	}

	return nil
}

// fetchEmailsFromFolder is a helper to fetch all emails from a specific folder
func (c *Client) fetchEmailsFromFolder(folder string) ([]Email, error) {
	client, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	mbox, err := client.Select(folder, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	// If folder is empty, return nil
	if mbox.NumMessages == 0 {
		return nil, nil
	}

	// Fetch all messages by sequence number (more reliable than search)
	var seqSet imap.SeqSet
	seqSet.AddRange(1, mbox.NumMessages)

	fetchOptions := &imap.FetchOptions{
		UID:      true,
		Flags:    true,
		Envelope: true,
	}

	fetchCmd := client.Fetch(seqSet, fetchOptions)

	var emails []Email
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		msgData, err := msg.Collect()
		if err != nil {
			log.Printf("Error collecting message: %v", err)
			continue
		}

		email := Email{
			UID:   uint32(msgData.UID),
			Flags: flagsToStrings(msgData.Flags),
		}

		if msgData.Envelope != nil {
			email.MessageID = msgData.Envelope.MessageID
			email.Subject = msgData.Envelope.Subject
			if len(msgData.Envelope.From) > 0 {
				from := msgData.Envelope.From[0]
				email.From = fmt.Sprintf("%s@%s", from.Mailbox, from.Host)
			}
		}

		emails = append(emails, email)
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	return emails, nil
}

// deleteEmailsFromFolder deletes emails by UID from a specific folder
func (c *Client) deleteEmailsFromFolder(folder string, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}

	client, err := c.connect()
	if err != nil {
		return err
	}
	defer client.Close()

	_, err = client.Select(folder, nil).Wait()
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	imapUIDs := make([]imap.UID, len(uids))
	for i, uid := range uids {
		imapUIDs[i] = imap.UID(uid)
	}

	uidSet := imap.UIDSetNum(imapUIDs...)

	storeCmd := client.Store(uidSet, &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagDeleted},
	}, nil)

	if err := storeCmd.Close(); err != nil {
		return fmt.Errorf("failed to mark as deleted: %w", err)
	}

	if err := client.Expunge().Close(); err != nil {
		return fmt.Errorf("failed to expunge: %w", err)
	}

	return nil
}

// FetchEmailsFromBlockFolder returns all emails in the USPIS/Block folder
func (c *Client) FetchEmailsFromBlockFolder() ([]Email, error) {
	return c.fetchEmailsFromFolder(FolderBlock)
}

// DeleteEmailsFromBlockFolder deletes emails by UID from the USPIS/Block folder
func (c *Client) DeleteEmailsFromBlockFolder(uids []uint32) error {
	return c.deleteEmailsFromFolder(FolderBlock, uids)
}

// FetchEmailsFromTransactionalOnlyFolder returns all emails in the USPIS/Transactional Only folder
func (c *Client) FetchEmailsFromTransactionalOnlyFolder() ([]Email, error) {
	return c.fetchEmailsFromFolder(FolderTransactionalOnly)
}

// DeleteEmailsFromTransactionalOnlyFolder deletes emails by UID from the USPIS/Transactional Only folder
func (c *Client) DeleteEmailsFromTransactionalOnlyFolder(uids []uint32) error {
	return c.deleteEmailsFromFolder(FolderTransactionalOnly, uids)
}

// CreateBlockFolderIfNotExists ensures the USPIS folder structure exists (alias for backwards compat)
func (c *Client) CreateBlockFolderIfNotExists() error {
	return c.CreateUSPISFolders()
}

// FetchEmailsFromSenders returns emails from specific sender addresses in a folder
func (c *Client) FetchEmailsFromSenders(folder string, senders []string) ([]Email, error) {
	if len(senders) == 0 {
		return nil, nil
	}

	client, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	_, err = client.Select(folder, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	var allEmails []Email

	for _, sender := range senders {
		searchCmd := client.Search(&imap.SearchCriteria{
			Header: []imap.SearchCriteriaHeaderField{
				{Key: "From", Value: sender},
			},
		}, nil)

		searchData, err := searchCmd.Wait()
		if err != nil {
			log.Printf("Search for sender %s failed: %v", sender, err)
			continue
		}

		if len(searchData.AllUIDs()) == 0 {
			continue
		}

		fetchOptions := &imap.FetchOptions{
			UID:      true,
			Flags:    true,
			Envelope: true,
		}

		uidSet := imap.UIDSetNum(searchData.AllUIDs()...)
		fetchCmd := client.Fetch(uidSet, fetchOptions)

		for {
			msg := fetchCmd.Next()
			if msg == nil {
				break
			}

			msgData, err := msg.Collect()
			if err != nil {
				continue
			}

			email := Email{
				UID:   uint32(msgData.UID),
				Flags: flagsToStrings(msgData.Flags),
			}

			if msgData.Envelope != nil {
				email.MessageID = msgData.Envelope.MessageID
				email.Subject = msgData.Envelope.Subject
				if len(msgData.Envelope.From) > 0 {
					from := msgData.Envelope.From[0]
					email.From = fmt.Sprintf("%s@%s", from.Mailbox, from.Host)
				}
			}

			allEmails = append(allEmails, email)
		}

		fetchCmd.Close()
	}

	return allEmails, nil
}

// DeleteEmails deletes emails by UID from a folder
func (c *Client) DeleteEmails(folder string, uids []uint32) error {
	return c.deleteEmailsFromFolder(folder, uids)
}

// FolderEmails holds emails found in a specific folder
type FolderEmails struct {
	Folder string
	Emails []Email
}

// ScanFoldersForSenders searches multiple folders for emails from specific senders using a single connection
func (c *Client) ScanFoldersForSenders(folders []string, senders []string) ([]FolderEmails, error) {
	if len(senders) == 0 || len(folders) == 0 {
		return nil, nil
	}

	client, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Build a set of senders for fast lookup (lowercase)
	senderSet := make(map[string]bool)
	for _, s := range senders {
		senderSet[strings.ToLower(s)] = true
	}

	var results []FolderEmails
	var foldersScanned, totalEmails int

	for _, folder := range folders {
		mbox, err := client.Select(folder, nil).Wait()
		if err != nil {
			log.Printf("Failed to select folder %s: %v", folder, err)
			continue
		}
		foldersScanned++

		// Skip empty folders
		if mbox.NumMessages == 0 {
			continue
		}

		// Fetch all emails in folder (just envelope - lightweight)
		var seqSet imap.SeqSet
		seqSet.AddRange(1, mbox.NumMessages)

		fetchOptions := &imap.FetchOptions{
			UID:      true,
			Flags:    true,
			Envelope: true,
		}

		fetchCmd := client.Fetch(seqSet, fetchOptions)

		var folderEmails []Email
		for {
			msg := fetchCmd.Next()
			if msg == nil {
				break
			}

			msgData, err := msg.Collect()
			if err != nil {
				continue
			}

			// Extract sender email
			var fromEmail string
			if msgData.Envelope != nil && len(msgData.Envelope.From) > 0 {
				from := msgData.Envelope.From[0]
				fromEmail = strings.ToLower(fmt.Sprintf("%s@%s", from.Mailbox, from.Host))
			}

			// Check if sender is in our list
			if fromEmail != "" && senderSet[fromEmail] {
				email := Email{
					UID:   uint32(msgData.UID),
					Flags: flagsToStrings(msgData.Flags),
					From:  fromEmail,
				}
				if msgData.Envelope != nil {
					email.MessageID = msgData.Envelope.MessageID
					email.Subject = msgData.Envelope.Subject
				}
				folderEmails = append(folderEmails, email)
			}
			totalEmails++
		}

		if err := fetchCmd.Close(); err != nil {
			log.Printf("Error fetching from %s: %v", folder, err)
		}

		if len(folderEmails) > 0 {
			results = append(results, FolderEmails{
				Folder: folder,
				Emails: folderEmails,
			})
		}
	}

	log.Printf("Scan complete: checked %d emails across %d folders, %d folders had matches", totalEmails, foldersScanned, len(results))
	return results, nil
}

// DeleteEmailsFromFolders deletes emails from multiple folders using a single connection
func (c *Client) DeleteEmailsFromFolders(folderUIDs map[string][]uint32) error {
	if len(folderUIDs) == 0 {
		return nil
	}

	client, err := c.connect()
	if err != nil {
		return err
	}
	defer client.Close()

	for folder, uids := range folderUIDs {
		if len(uids) == 0 {
			continue
		}

		_, err = client.Select(folder, nil).Wait()
		if err != nil {
			log.Printf("Failed to select folder %s for deletion: %v", folder, err)
			continue
		}

		imapUIDs := make([]imap.UID, len(uids))
		for i, uid := range uids {
			imapUIDs[i] = imap.UID(uid)
		}

		uidSet := imap.UIDSetNum(imapUIDs...)

		storeCmd := client.Store(uidSet, &imap.StoreFlags{
			Op:    imap.StoreFlagsAdd,
			Flags: []imap.Flag{imap.FlagDeleted},
		}, nil)

		if err := storeCmd.Close(); err != nil {
			log.Printf("Failed to mark as deleted in %s: %v", folder, err)
			continue
		}

		if err := client.Expunge().Close(); err != nil {
			log.Printf("Failed to expunge in %s: %v", folder, err)
			continue
		}

		log.Printf("Deleted %d emails from %s", len(uids), folder)
	}

	return nil
}

// FetchFullEmailsByUIDs fetches full email content for specific UIDs in a folder
func (c *Client) FetchFullEmailsByUIDs(folder string, uids []uint32) ([]FetchedEmail, error) {
	if len(uids) == 0 {
		return nil, nil
	}

	client, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	_, err = client.Select(folder, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	imapUIDs := make([]imap.UID, len(uids))
	for i, uid := range uids {
		imapUIDs[i] = imap.UID(uid)
	}

	uidSet := imap.UIDSetNum(imapUIDs...)

	fetchOptions := &imap.FetchOptions{
		UID:         true,
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}

	fetchCmd := client.Fetch(uidSet, fetchOptions)

	var emails []FetchedEmail
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		msgData, err := msg.Collect()
		if err != nil {
			log.Printf("Error collecting message: %v", err)
			continue
		}

		email := FetchedEmail{
			UID: uint32(msgData.UID),
		}

		if msgData.Envelope != nil {
			email.MessageID = msgData.Envelope.MessageID
			email.Subject = msgData.Envelope.Subject
			if !msgData.Envelope.Date.IsZero() {
				email.Date = msgData.Envelope.Date.Format("2006-01-02 15:04:05")
			}
			if len(msgData.Envelope.From) > 0 {
				from := msgData.Envelope.From[0]
				email.From = fmt.Sprintf("%s@%s", from.Mailbox, from.Host)
			}
			if len(msgData.Envelope.To) > 0 {
				var tos []string
				for _, to := range msgData.Envelope.To {
					tos = append(tos, fmt.Sprintf("%s@%s", to.Mailbox, to.Host))
				}
				email.To = strings.Join(tos, ", ")
			}
		}

		// Parse body content
		for _, section := range msgData.BodySection {
			if len(section.Bytes) == 0 {
				continue
			}
			parsed, parseErr := mail.ReadMessage(bytes.NewReader(section.Bytes))
			if parseErr != nil {
				log.Printf("Error parsing message: %v", parseErr)
				continue
			}

			var headerLines []string
			for key, values := range parsed.Header {
				for _, value := range values {
					headerLines = append(headerLines, fmt.Sprintf("%s: %s", key, value))
				}
			}
			email.Headers = strings.Join(headerLines, "\n")

			bodyText, bodyHTML, hasAttachments := parseEmailBody(parsed)
			email.BodyText = bodyText
			email.BodyHTML = bodyHTML
			email.HasAttachments = hasAttachments
			break
		}

		emails = append(emails, email)
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	return emails, nil
}

func flagsToStrings(flags []imap.Flag) []string {
	result := make([]string, len(flags))
	for i, f := range flags {
		result[i] = string(f)
	}
	return result
}

// ParseEmailAddress extracts the email address from a From header
func ParseEmailAddress(from string) string {
	if from == "" {
		return ""
	}

	addr, err := mail.ParseAddress(from)
	if err == nil {
		return strings.ToLower(addr.Address)
	}

	return strings.ToLower(strings.TrimSpace(from))
}

// FetchRecentEmailsWithFlags fetches the N most recent emails with all their flags for diagnostics
func (c *Client) FetchRecentEmailsWithFlags(count int) ([]Email, error) {
	client, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	mbox, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	if mbox.NumMessages == 0 {
		return nil, nil
	}

	start := uint32(1)
	if mbox.NumMessages > uint32(count) {
		start = mbox.NumMessages - uint32(count) + 1
	}

	var seqSet imap.SeqSet
	seqSet.AddRange(start, mbox.NumMessages)

	fetchOptions := &imap.FetchOptions{
		UID:      true,
		Flags:    true,
		Envelope: true,
	}

	fetchCmd := client.Fetch(seqSet, fetchOptions)

	var emails []Email
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		msgData, err := msg.Collect()
		if err != nil {
			log.Printf("Error collecting message: %v", err)
			continue
		}

		email := Email{
			UID:   uint32(msgData.UID),
			Flags: flagsToStrings(msgData.Flags),
		}

		if msgData.Envelope != nil {
			email.MessageID = msgData.Envelope.MessageID
			email.Subject = msgData.Envelope.Subject
			if len(msgData.Envelope.From) > 0 {
				from := msgData.Envelope.From[0]
				email.From = fmt.Sprintf("%s@%s", from.Mailbox, from.Host)
			}
		}

		emails = append(emails, email)
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	return emails, nil
}

// FetchFullEmailsFromFolder fetches emails with full body content from a folder
func (c *Client) FetchFullEmailsFromFolder(folder string) ([]FetchedEmail, error) {
	client, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	mbox, err := client.Select(folder, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	if mbox.NumMessages == 0 {
		return nil, nil
	}

	var seqSet imap.SeqSet
	seqSet.AddRange(1, mbox.NumMessages)

	fetchOptions := &imap.FetchOptions{
		UID:         true,
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}

	fetchCmd := client.Fetch(seqSet, fetchOptions)

	var emails []FetchedEmail
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		msgData, err := msg.Collect()
		if err != nil {
			log.Printf("Error collecting message: %v", err)
			continue
		}

		email := FetchedEmail{
			UID: uint32(msgData.UID),
		}

		if msgData.Envelope != nil {
			email.MessageID = msgData.Envelope.MessageID
			email.Subject = msgData.Envelope.Subject
			if !msgData.Envelope.Date.IsZero() {
				email.Date = msgData.Envelope.Date.Format("2006-01-02 15:04:05")
			}
			if len(msgData.Envelope.From) > 0 {
				from := msgData.Envelope.From[0]
				email.From = fmt.Sprintf("%s@%s", from.Mailbox, from.Host)
			}
			if len(msgData.Envelope.To) > 0 {
				var tos []string
				for _, to := range msgData.Envelope.To {
					tos = append(tos, fmt.Sprintf("%s@%s", to.Mailbox, to.Host))
				}
				email.To = strings.Join(tos, ", ")
			}
		}

		// Parse body content (BodySection is []FetchBodySectionBuffer)
		for _, section := range msgData.BodySection {
			if len(section.Bytes) == 0 {
				continue
			}
			parsed, parseErr := mail.ReadMessage(bytes.NewReader(section.Bytes))
			if parseErr != nil {
				log.Printf("Error parsing message: %v", parseErr)
				continue
			}

			// Extract headers
			var headerLines []string
			for key, values := range parsed.Header {
				for _, value := range values {
					headerLines = append(headerLines, fmt.Sprintf("%s: %s", key, value))
				}
			}
			email.Headers = strings.Join(headerLines, "\n")

			// Parse body
			bodyText, bodyHTML, hasAttachments := parseEmailBody(parsed)
			email.BodyText = bodyText
			email.BodyHTML = bodyHTML
			email.HasAttachments = hasAttachments
			break // Only process first body section
		}

		emails = append(emails, email)
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	return emails, nil
}

// FetchFullEmailsFromBlockFolder returns full emails from the USPIS/Block folder
func (c *Client) FetchFullEmailsFromBlockFolder() ([]FetchedEmail, error) {
	return c.FetchFullEmailsFromFolder(FolderBlock)
}

// FetchFullEmailsFromTransactionalOnlyFolder returns full emails from the USPIS/Transactional Only folder
func (c *Client) FetchFullEmailsFromTransactionalOnlyFolder() ([]FetchedEmail, error) {
	return c.FetchFullEmailsFromFolder(FolderTransactionalOnly)
}

// parseEmailBody extracts text and HTML body from an email, and detects attachments
func parseEmailBody(msg *mail.Message) (bodyText, bodyHTML string, hasAttachments bool) {
	contentType := msg.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// Try to read body as plain text
		body, _ := io.ReadAll(msg.Body)
		return string(body), "", false
	}

	if strings.HasPrefix(mediaType, "text/plain") {
		body, _ := io.ReadAll(msg.Body)
		return string(body), "", false
	}

	if strings.HasPrefix(mediaType, "text/html") {
		body, _ := io.ReadAll(msg.Body)
		return "", string(body), false
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			body, _ := io.ReadAll(msg.Body)
			return string(body), "", false
		}

		reader := multipart.NewReader(msg.Body, boundary)
		return parseMultipart(reader)
	}

	// For other content types (like application/octet-stream), treat as attachment
	return "", "", true
}

// parseMultipart recursively parses multipart content
func parseMultipart(reader *multipart.Reader) (bodyText, bodyHTML string, hasAttachments bool) {
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		contentType := part.Header.Get("Content-Type")
		contentDisposition := part.Header.Get("Content-Disposition")

		// Check if this is an attachment
		if strings.Contains(contentDisposition, "attachment") {
			hasAttachments = true
			continue
		}

		mediaType, params, _ := mime.ParseMediaType(contentType)

		if strings.HasPrefix(mediaType, "text/plain") && bodyText == "" {
			body, _ := io.ReadAll(part)
			bodyText = string(body)
		} else if strings.HasPrefix(mediaType, "text/html") && bodyHTML == "" {
			body, _ := io.ReadAll(part)
			bodyHTML = string(body)
		} else if strings.HasPrefix(mediaType, "multipart/") {
			boundary := params["boundary"]
			if boundary != "" {
				subReader := multipart.NewReader(part, boundary)
				subText, subHTML, subAttach := parseMultipart(subReader)
				if bodyText == "" {
					bodyText = subText
				}
				if bodyHTML == "" {
					bodyHTML = subHTML
				}
				if subAttach {
					hasAttachments = true
				}
			}
		} else if contentDisposition != "" || !strings.HasPrefix(mediaType, "text/") {
			// Non-text parts without explicit attachment disposition
			// could be inline images, etc.
			hasAttachments = true
		}
	}
	return
}
