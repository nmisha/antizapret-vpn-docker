# dns-guard

`dns-guard` watches AdGuard `querylog.json`, matches risky domains, accumulates score per profile, sends notifications, and can schedule profile blocking.

## How It Works

- Reads only new lines from `querylog.json`.
- On service start, syncs cursor to the end of the file and does not replay history.
- Resolves profile owner by source IP via WG, AWG, or OVPN APIs.
- Adds matched rule risk into per-profile minute buckets.
- Keeps only the last 24 hours of buckets in `state.json`.
- Calculates:
  - `score15m`: sum of buckets for the last 15 minutes
  - `score24h`: sum of buckets for the last 24 hours
  - effective score: `max(score15m, score24h)`

## Notifications

Notification is sent when all conditions are true:

- effective score is `>= score_notify_at`
- effective score is higher than the previous notified score
- notification cooldown has passed

Optional ETA can be included in the notification:

- `predict_block_eta: true`
- ETA is only an estimate based on recent score growth

## Blocking

Blocking thresholds are separate from notification threshold:

- `score_block_at_15m`
- `score_block_at_24h`

If one of them is crossed:

- service sends a `block_pending` warning immediately
- stores pending block in `state.json`
- performs actual block after `block_delay_seconds`

Actual blocking is supported for:

- `wg` if `wg.block=true`
- `awg` if `awg.block=true`

OVPN can be monitored, but blocking is disabled unless implemented and enabled separately.

## Config

Main fields in `config-mine/dns-guard/config.json`:

- `enabled`: global on/off switch
- `poll_interval_seconds`: how often to read new query log events and recalculate score
- `querylog_path`: path to AdGuard `querylog.json`
- `state_path`: path to `state.json`
- `notification_cooldown_seconds`: cooldown for repeated score notifications
- `min_rule_risk`: lower clamp for `risk` loaded from rules file
- `max_rule_risk`: upper clamp for `risk` loaded from rules file
- `score_notify_at`: notification threshold
- `predict_block_eta`: include estimated time to block in notifications
- `score_block_at_15m`: blocking threshold for 15-minute window
- `score_block_at_24h`: blocking threshold for 24-hour window
- `block_delay_seconds`: delay between block warning and actual block
- `ignore_ips`: IPs to ignore completely
- `subnets`: monitored client subnets by profile type

Delivery settings:

- `notification_api_url` and `notification_api_token` can be passed via env

## Rules

Rules are stored in `config-mine/dns-guard/risk-domains.json`.

Each rule has:

- `domain`
- `match`: `exact` or `suffix`
- `risk`: integer score added for each matched event
- effective rule risk is clamped to `min_rule_risk..max_rule_risk`
- `reason`
- `enabled`
- `created_at`

Rule examples:

- `risk: 3-4`: low or medium-risk telemetry / suspicious service
- `risk: 5-7`: serious suspicious domain
- `risk: 8-9`: malware / phishing / hard block candidate

## Practical Starting Thresholds

Conservative starting point:

- `score_notify_at: 12`
- `score_block_at_15m: 18`
- `score_block_at_24h: 36`
- `block_delay_seconds: 30`

This means:

- a single `risk: 9` hit does not block immediately
- repeated `risk: 9` events in a short time can trigger block
- lower-risk repeated activity can still accumulate into warning or block

## State

`state.json` stores:

- query cursor
- dedup keys for recent events
- per-profile minute buckets
- last notified score
- pending block state
- last matched domain / reason

If `dns-guard` restarts:

- it does not replay old log history
- it resumes from current end of `querylog.json`
- existing `state.json` is used to preserve buckets and pending blocks

## Notes

- Large query log tails should not blow up memory: processing is streaming, not batch-loading into slices.
- Large tails can still increase one poll cycle duration because the file is processed line by line.
- Pending blocks are checked every second independently from `poll_interval_seconds`.
