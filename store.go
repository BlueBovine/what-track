package main

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type LogStore struct{ db *sql.DB }

func OpenLogStore(path string) (*LogStore, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS presence_log (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		ts        TIMESTAMP NOT NULL,
		contact   TEXT NOT NULL,
		number    TEXT NOT NULL,
		status    TEXT NOT NULL,      -- online | offline
		last_seen TIMESTAMP           -- NULL when hidden/unknown
	);
	CREATE INDEX IF NOT EXISTS idx_presence_contact_ts ON presence_log(contact, ts);`)
	if err != nil {
		return nil, err
	}
	return &LogStore{db: db}, nil
}

func (s *LogStore) Add(c Contact, status string, lastSeen time.Time) error {
	var ls any
	if !lastSeen.IsZero() {
		ls = lastSeen.UTC()
	}
	_, err := s.db.Exec(`INSERT INTO presence_log(ts, contact, number, status, last_seen) VALUES (?,?,?,?,?)`,
		time.Now().UTC(), c.Name, c.Number, status, ls)
	return err
}
