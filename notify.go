package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Notifier struct {
	server, topic, token string
}

func (n *Notifier) Enabled() bool { return n.topic != "" }

func (n *Notifier) Send(ctx context.Context, title, body string) error {
	return n.send(ctx, title, body, "")
}

// Alert sends a high-priority notification for service health problems.
func (n *Notifier) Alert(ctx context.Context, title, body string) error {
	return n.send(ctx, title, body, "high")
}

func (n *Notifier) send(ctx context.Context, title, body, priority string) error {
	if !n.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(n.server, "/")+"/"+n.topic, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	if priority != "" {
		req.Header.Set("Priority", priority)
		req.Header.Set("Tags", "warning")
	}
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: %s", resp.Status)
	}
	return nil
}
