package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	IMAPServer   string
	IMAPPort     int
	Email        string
	AppPassword  string
	PollInterval time.Duration
	WebPort      int
	DBPath       string
}

func Load() (*Config, error) {
	email := os.Getenv("ICLOUD_EMAIL")
	if email == "" {
		return nil, fmt.Errorf("ICLOUD_EMAIL environment variable is required")
	}

	appPassword := os.Getenv("ICLOUD_APP_PASSWORD")
	if appPassword == "" {
		return nil, fmt.Errorf("ICLOUD_APP_PASSWORD environment variable is required")
	}

	pollInterval := 1 * time.Minute
	if intervalStr := os.Getenv("POLL_INTERVAL"); intervalStr != "" {
		if parsed, err := time.ParseDuration(intervalStr); err == nil {
			pollInterval = parsed
		}
	}

	webPort := 8080
	if portStr := os.Getenv("WEB_PORT"); portStr != "" {
		if parsed, err := strconv.Atoi(portStr); err == nil {
			webPort = parsed
		}
	}

	dbPath := "/data/postal.db"
	if path := os.Getenv("DB_PATH"); path != "" {
		dbPath = path
	}

	return &Config{
		IMAPServer:   "imap.mail.me.com",
		IMAPPort:     993,
		Email:        email,
		AppPassword:  appPassword,
		PollInterval: pollInterval,
		WebPort:      webPort,
		DBPath:       dbPath,
	}, nil
}
