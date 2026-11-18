# what-track

Logs (and optionally notifies on) WhatsApp contacts' online / last-seen changes.
Go + [whatsmeow](https://github.com/tulir/whatsmeow), runs in Docker.

## Why this exists

Some of my friends and family aren't great with messaging apps. They'd rather talk
by phone, but they keep their phones on silent and rarely pick up. This small service
notifies me when one of them opens WhatsApp, so that if I genuinely need to reach them,
I know it's a good moment to call.

## Privacy and intended use

This tool is meant for **personal, consensual, low-stakes use**: a handful of people
you know well, who would be unsurprised and unbothered to learn you get a ping when
they come online. Please keep it that way.

- **Only track people who know about it.** Tell them, and stop if they object.
- **Never use it to monitor, stalk, control or pressure anyone.** That includes
  partners, ex-partners, colleagues or strangers. It is not a way to check who
  someone is talking to, or when they are active.
- **It only sees what WhatsApp already shows you.** Contacts who hide "last seen &
  online" produce no events. Respect that choice; do not try to work around it.
- **Keep the data private.** `data/presence.db` is a record of people's habits, and
  `data/session.db` grants access to your WhatsApp account. Don't share or publish
  either, and delete history you no longer need.
- **Track the minimum.** Prefer a short contact list, and `notify: false` for anyone
  you only want logged.

## A note for privacy advocates

Whether someone's online status is visible is entirely in that person's own hands.
WhatsApp's privacy settings let each user choose who can see their "last seen &
online" (everyone, only contacts, selected contacts, or nobody), and this tool can
see nothing beyond what those settings allow. It cannot override them.

WhatsApp's privacy controls have improved a lot over the years, and I'd encourage
everyone to visit **Settings > Privacy** in the app and tailor the options (last
seen & online, profile photo, about, read receipts, who can add you to groups, and
more) to whatever makes them feel comfortable. If you'd rather not appear in tools
like this, you can restrict "Last seen & online" to fewer people, exclude particular
contacts (for example with **My contacts except...**), or choose **Nobody**. It's your
call, and no explanation is needed.

## Caveats

- **The linked account appears online all the time.** WhatsApp only delivers contact
  presence while your own account is marked available, so while this service runs, the
  person whose device is linked will show as online to anyone allowed to see their
  status. Bear that in mind, particularly if you rely on your own "last seen & online"
  to signal when you're around. Also, if you hide your own last seen, WhatsApp hides
  other people's from you.
- Unofficial client: it violates WhatsApp's ToS and could get the account restricted,
  so a secondary number is advisable.

## Setup
    cp config.example.yaml data/config.yaml   # edit contacts / ntfy topic
    docker compose run --rm what-track         # first run: scan QR, Ctrl+C after "connected"
    docker compose up -d                       # run as a service

## Query the log
    sqlite3 data/presence.db "select ts,contact,status,last_seen from presence_log order by id desc limit 20"

`data/session.db` holds your WhatsApp credentials; keep it private.


## Config notes
- `number`: international format with country code first, no leading `0`/`00`
  (e.g. UK `07700 900123` -> `447700900123`). Spaces, `+` and dashes are stripped automatically.
- `name` is only a label for logs and notifications; it need not match anything in WhatsApp.

## Logs
- Runtime output: `docker compose logs -f what-track`
- Presence history: `data/presence.db`, table `presence_log` (ts, contact, number, status, last_seen)

## Troubleshooting
- `can't send presence without PushName set` right after pairing: harmless, it retries
  automatically once the push name arrives.
- You should see `subscribed to <name> (<number>@s.whatsapp.net)` per contact on connect.
  If the number looks wrong, fix `config.yaml`.
- Subscribed but no `presence event` lines: the contact likely hides "last seen & online"
  from you, or you hide yours (WhatsApp then hides theirs too). Check both privacy settings.
- `presence event from ...@lid` but nothing tracked: LID-to-number mapping failed; open an issue
  with the log line.

## Health alerts (need an ntfy topic)
- **Device unlinked / client outdated:** high-priority alert, then the process exits non-zero.
  Docker restarts it into the QR pairing flow; attach with `docker compose logs -f what-track`
  or run `docker compose run --rm what-track` to scan the QR. Outdated client: update whatsmeow
  and rebuild.
- **Disconnected longer than `disconnect_alert_after`** (default 15m): high-priority alert, plus a
  "back online" message on recovery.
