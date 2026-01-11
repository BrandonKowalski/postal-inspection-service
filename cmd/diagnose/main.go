package main

import (
	"fmt"
	"log"
	"strings"

	"crypto/tls"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"postal-inspection-service/internal/config"
)

func main() {
	fmt.Println("=== USPIS - Postal Inspection Service Diagnostics ===")
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Connecting to %s:%d as %s...\n", cfg.IMAPServer, cfg.IMAPPort, cfg.Email)

	addr := fmt.Sprintf("%s:%d", cfg.IMAPServer, cfg.IMAPPort)
	client, err := imapclient.DialTLS(addr, &imapclient.Options{
		TLSConfig: &tls.Config{ServerName: cfg.IMAPServer},
	})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if err := client.Login(cfg.Email, cfg.AppPassword).Wait(); err != nil {
		log.Fatalf("Failed to login: %v", err)
	}

	// Check each USPIS folder
	folders := []string{"USPIS/Block", "USPIS/Transactional Only"}

	for _, folder := range folders {
		fmt.Printf("\n--- Checking folder: %s ---\n", folder)

		mbox, err := client.Select(folder, nil).Wait()
		if err != nil {
			fmt.Printf("Error selecting folder: %v\n", err)
			continue
		}

		fmt.Printf("Folder stats: %d messages\n", mbox.NumMessages)

		if mbox.NumMessages == 0 {
			fmt.Println("Folder is empty.")
			continue
		}

		// Fetch all messages by sequence number
		var seqSet imap.SeqSet
		seqSet.AddRange(1, mbox.NumMessages)

		fetchOptions := &imap.FetchOptions{
			UID:      true,
			Flags:    true,
			Envelope: true,
		}

		fetchCmd := client.Fetch(seqSet, fetchOptions)

		count := 0
		for {
			msg := fetchCmd.Next()
			if msg == nil {
				break
			}

			msgData, err := msg.Collect()
			if err != nil {
				fmt.Printf("Error collecting message: %v\n", err)
				continue
			}

			count++
			subject := ""
			from := ""
			if msgData.Envelope != nil {
				subject = msgData.Envelope.Subject
				if len(msgData.Envelope.From) > 0 {
					from = fmt.Sprintf("%s@%s", msgData.Envelope.From[0].Mailbox, msgData.Envelope.From[0].Host)
				}
			}

			fmt.Printf("\n[%d] UID: %d\n", count, msgData.UID)
			fmt.Printf("    Subject: %s\n", subject)
			fmt.Printf("    From: %s\n", from)
			fmt.Printf("    Flags: %v\n", msgData.Flags)
		}

		if err := fetchCmd.Close(); err != nil {
			fmt.Printf("Fetch error: %v\n", err)
		}

		if count == 0 {
			fmt.Println("No messages fetched.")
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("\nHow to use USPIS:")
	fmt.Println("  Move emails to USPIS/Block to block senders")
	fmt.Println("  Move emails to USPIS/Transactional Only for marketing filtering")
}
