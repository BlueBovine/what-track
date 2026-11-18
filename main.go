package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type tracker struct {
	cfg    *Config
	client *whatsmeow.Client
	logs   *LogStore
	ntfy   *Notifier
	byUser map[string]Contact // JID user part (phone number) -> contact

	mu   sync.Mutex
	last map[string]string // contact number -> last logged state key

	downTimer *time.Timer // fires the "disconnected too long" alert
	downSince time.Time
	alerted   bool
}

func main() {
	cfgPath := flag.String("config", "data/config.yaml", "config file")
	flag.Parse()

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logs, err := OpenLogStore(cfg.LogDB)
	if err != nil {
		log.Fatalf("log db: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	container, err := sqlstore.New(ctx, "sqlite",
		"file:"+cfg.SessionDB+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
		waLog.Stdout("DB", "WARN", true))
	if err != nil {
		log.Fatalf("session db: %v", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		log.Fatalf("device: %v", err)
	}

	t := &tracker{
		cfg:    cfg,
		client: whatsmeow.NewClient(device, waLog.Stdout("WA", "WARN", true)),
		logs:   logs,
		ntfy:   &Notifier{cfg.Ntfy.Server, cfg.Ntfy.Topic, cfg.Ntfy.Token},
		byUser: map[string]Contact{},
		last:   map[string]string{},
	}
	for _, c := range cfg.Contacts {
		t.byUser[c.Number] = c
	}
	t.client.AddEventHandler(t.handle)

	if t.client.Store.ID == nil {
		qrChan, _ := t.client.GetQRChannel(ctx)
		if err := t.client.Connect(); err != nil {
			log.Fatalf("connect: %v", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("Scan this QR: WhatsApp > Settings > Linked devices > Link a device")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				log.Printf("pairing: %s", evt.Event)
			}
		}
		if t.client.Store.ID == nil {
			log.Fatal("pairing did not complete (QR expired); exiting so it can be restarted")
		}
	} else if err := t.client.Connect(); err != nil {
		log.Fatalf("connect: %v", err)
	}

	ticker := time.NewTicker(cfg.ResubscribeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			t.client.Disconnect()
			return
		case <-ticker.C:
			t.subscribeAll(ctx)
		}
	}
}

func (t *tracker) subscribeAll(ctx context.Context) {
	if !t.client.IsConnected() || !t.client.IsLoggedIn() {
		return
	}
	// Presence is only delivered while we are marked available ourselves.
	// Right after pairing the push name is not set yet; the PushName event retries.
	if err := t.client.SendPresence(ctx, types.PresenceAvailable); err != nil {
		log.Printf("send presence: %v (will retry)", err)
		return
	}
	for _, c := range t.cfg.Contacts {
		jid := types.NewJID(c.Number, types.DefaultUserServer)
		if err := t.client.SubscribePresence(ctx, jid); err != nil {
			log.Printf("subscribe %s (%s): %v", c.Name, c.Number, err)
		} else {
			log.Printf("subscribed to %s (%s)", c.Name, jid)
		}
	}
}

func (t *tracker) handle(raw any) {
	switch e := raw.(type) {
	case *events.Connected:
		log.Println("connected; subscribing to contacts")
		t.onUp()
		t.subscribeAll(context.Background())
	case *events.PushNameSetting:
		t.subscribeAll(context.Background())
	case *events.Disconnected:
		log.Println("disconnected (auto-reconnect enabled)")
		t.onDown()
	case *events.LoggedOut:
		log.Println("logged out / device unlinked; re-pairing required")
		t.fatalAlert("WhatsApp tracker: device unlinked",
			"WhatsApp unlinked this device. Restart the service and scan the QR code again (docker compose run --rm what-track).")
	case *events.ClientOutdated:
		log.Println("client outdated; whatsmeow must be updated")
		t.fatalAlert("WhatsApp tracker: client outdated",
			"WhatsApp rejected this client version. Update whatsmeow (go get -u go.mau.fi/whatsmeow) and rebuild.")
	case *events.Presence:
		log.Printf("presence event from %s unavailable=%v lastseen=%v", e.From, e.Unavailable, e.LastSeen)
		t.onPresence(e)
	}
}

func (t *tracker) resolve(jid types.JID) (Contact, bool) {
	if jid.Server == types.HiddenUserServer { // LID -> phone number
		pn, err := t.client.Store.LIDs.GetPNForLID(context.Background(), jid)
		if err != nil || pn.IsEmpty() {
			return Contact{}, false
		}
		jid = pn
	}
	c, ok := t.byUser[jid.User]
	return c, ok
}

func (t *tracker) onPresence(e *events.Presence) {
	c, ok := t.resolve(e.From)
	if !ok {
		return
	}
	status := "online"
	if e.Unavailable {
		status = "offline"
	}
	key := status + "|" + e.LastSeen.UTC().Format(time.RFC3339)

	t.mu.Lock()
	same := t.last[c.Number] == key
	t.last[c.Number] = key
	t.mu.Unlock()
	if same {
		return
	}

	msg := fmt.Sprintf("%s is %s", c.Name, status)
	if !e.LastSeen.IsZero() {
		msg += fmt.Sprintf(" (last seen %s)", e.LastSeen.Local().Format("2006-01-02 15:04:05"))
	}
	log.Println(msg)
	if err := t.logs.Add(c, status, e.LastSeen); err != nil {
		log.Printf("log write: %v", err)
	}
	if c.Notify && t.ntfy.Enabled() {
		if err := t.ntfy.Send(context.Background(), "WhatsApp: "+c.Name, msg); err != nil {
			log.Printf("notify: %v", err)
		}
	}
}

// onDown starts the timer that alerts if we stay disconnected too long.
func (t *tracker) onDown() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.downTimer != nil { // already counting
		return
	}
	t.downSince = time.Now()
	t.downTimer = time.AfterFunc(t.cfg.DisconnectAlertAfter, func() {
		t.mu.Lock()
		t.alerted = true
		since := t.downSince
		t.mu.Unlock()
		msg := fmt.Sprintf("WhatsApp connection down since %s (%s). Tracking is paused.",
			since.Local().Format("15:04:05"), time.Since(since).Round(time.Minute))
		log.Println(msg)
		if err := t.ntfy.Alert(context.Background(), "WhatsApp tracker: disconnected", msg); err != nil {
			log.Printf("notify: %v", err)
		}
	})
}

// onUp cancels the pending alert and reports recovery if one was sent.
func (t *tracker) onUp() {
	t.mu.Lock()
	timer, alerted, since := t.downTimer, t.alerted, t.downSince
	t.downTimer, t.alerted = nil, false
	t.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if alerted {
		msg := fmt.Sprintf("WhatsApp connection restored after %s.", time.Since(since).Round(time.Minute))
		log.Println(msg)
		if err := t.ntfy.Send(context.Background(), "WhatsApp tracker: back online", msg); err != nil {
			log.Printf("notify: %v", err)
		}
	}
}

// fatalAlert notifies and exits non-zero so Docker restarts into the pairing flow.
func (t *tracker) fatalAlert(title, body string) {
	if err := t.ntfy.Alert(context.Background(), title, body); err != nil {
		log.Printf("notify: %v", err)
	}
	os.Exit(1)
}
