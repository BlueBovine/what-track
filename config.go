package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Contact struct {
	Name   string `yaml:"name"`
	Number string `yaml:"number"`
	Notify bool   `yaml:"notify"`
}

type Config struct {
	SessionDB string `yaml:"session_db"`
	LogDB     string `yaml:"log_db"`
	Ntfy      struct {
		Server string `yaml:"server"`
		Topic  string `yaml:"topic"`
		Token  string `yaml:"token"`
	} `yaml:"ntfy"`
	ResubscribeInterval  time.Duration `yaml:"resubscribe_interval"`
	DisconnectAlertAfter time.Duration `yaml:"disconnect_alert_after"`
	Contacts             []Contact     `yaml:"contacts"`
}

// digitsOnly strips spaces, '+', dashes, parentheses etc. WhatsApp JIDs need the
// bare international number (country code first, no leading 0 or 00).
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.SessionDB == "" {
		c.SessionDB = "session.db"
	}
	if c.LogDB == "" {
		c.LogDB = "presence.db"
	}
	if c.Ntfy.Server == "" {
		c.Ntfy.Server = "https://ntfy.sh"
	}
	if c.ResubscribeInterval <= 0 {
		c.ResubscribeInterval = 5 * time.Minute
	}
	if c.DisconnectAlertAfter <= 0 {
		c.DisconnectAlertAfter = 15 * time.Minute
	}
	for i := range c.Contacts {
		n := digitsOnly(c.Contacts[i].Number)
		if n == "" {
			return nil, fmt.Errorf("contact %q: number %q has no digits", c.Contacts[i].Name, c.Contacts[i].Number)
		}
		c.Contacts[i].Number = n
	}
	if len(c.Contacts) == 0 {
		return nil, fmt.Errorf("no contacts configured")
	}
	return &c, nil
}
