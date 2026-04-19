# dns-guard

`dns-guard` watches AdGuard query log, matches risky domains, accumulates score per profile, sends notifications, and can schedule profile blocking.

## How It Works

- Supports two query log sources:
  - AdGuard API via `GET /control/querylog`
  - direct file tailing of `querylog.json` for backward compatibility
- In API mode, reads recent pages from AdGuard and stops when it reaches already processed event time.
- In file mode, reads only new lines from `querylog.json`.
- On service start, syncs to current head and does not replay history.
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
- if the same profile later appears in query log again after block, its stored risk state is reset and score accumulation starts from zero

Actual blocking is supported for:

- `wg` if `wg.block=true`
- `awg` if `awg.block=true`

OVPN can be monitored, but blocking is disabled unless implemented and enabled separately.

## Config

Main fields in `config-mine/dns-guard/config.json`:

- `enabled`: global on/off switch
- `poll_interval_seconds`: how often to read new query log events and recalculate score
- `notification_cooldown_seconds`: cooldown for repeated score notifications
- `debug_log_enabled`: write detailed debug log next to `state.json`
- `track_skipped_events`: store selected skipped-event counters in `state.json`
- `history_catchup_enabled`: on startup or after source errors, try to backfill a limited amount of missed API history
- `history_catchup_max_age_minutes`: how far back catch-up is allowed to go
- `history_catchup_max_records`: max number of API querylog entries to process in one catch-up cycle
- `min_rule_risk`: lower clamp for `risk` loaded from rules file
- `max_rule_risk`: upper clamp for `risk` loaded from rules file
- `score_notify_at`: notification threshold
- `predict_block_eta`: include estimated time to block in notifications
- `score_block_at_15m`: blocking threshold for 15-minute window
- `score_block_at_24h`: blocking threshold for 24-hour window
- `block_delay_seconds`: delay between block warning and actual block
- `ignore_ips`: IPs to ignore completely
- `profile_whitelist`: profile names excluded from rule processing, grouped by type (`wg`, `ovpn`, `awg`)
- `subnets`: monitored client subnets by profile type

Delivery settings:

- restart-only env settings:
- `DNS_GUARD_STATE`: path to `state.json`
- `DNS_GUARD_NOTIFICATION_URL`
- `DNS_GUARD_NOTIFICATION_TOKEN`

Query log source settings:

- `querylog_source`: `api` or `file`
- for `api`:
  - `adguard.scheme`
  - `adguard.host`
  - `adguard.port`
  - `adguard.username`
  - `adguard.password`
  - `adguard.timeout_seconds`
  - `adguard.page_limit`
- for `file`:
  - `querylog_path`

Optional env overrides for API mode:

- `DNS_GUARD_AGH_SCHEME`
- `DNS_GUARD_AGH_HOST`
- `DNS_GUARD_AGH_PORT`
- `DNS_GUARD_AGH_USERNAME`
- `DNS_GUARD_AGH_PASSWORD`
- `DNS_GUARD_AGH_TIMEOUT_SECONDS`
- `DNS_GUARD_AGH_PAGE_LIMIT`

## Rules

Rules are stored in `config-mine/dns-guard/risk-domains.json`.

Each rule has:

- `domain` or `domains`
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

When `domains` is used, one logical rule is expanded into multiple domain matchers with the same `match`, `risk`, `reason`, and `enabled` settings.

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
- optional `skipped_events` counters when `track_skipped_events=true`

Tracked skipped-event reasons:

- `empty_ip`
- `ignored_ip`
- `duplicate_event`
- `resolve_error`
- `profile_not_found`
- `profile_whitelisted`

Skipped-event counters are reset every 36 hours.

If `dns-guard` restarts:

- it does not replay old log history
- in API mode it resumes from the current newest event returned by AdGuard
- in file mode it resumes from current end of `querylog.json`
- existing `state.json` is used to preserve buckets and pending blocks
- when `history_catchup_enabled=true`, API mode can backfill limited missed history on startup and after source errors
- catch-up history affects score and notifications, but does not schedule automatic blocking

## Debug Log

When `debug_log_enabled=true`, `dns-guard` appends a file named `dns-guard-debug.log` in the same directory as `state.json`.

The debug log includes:

- rule matches with profile, matched rule, and calculated scores
- `profile_not_found` details
- `resolve_error` details

## Notes

- API mode avoids delays from `querylog.json` flush latency because AdGuard serves in-memory entries too.
- File mode still exists as a fallback for backward compatibility.
- API mode may fetch multiple pages per cycle if many new events arrive between polls.
- Catch-up mode is bounded by both age and record count to avoid replaying too much history after restart or recovery.
- Automatic blocking is suppressed for catch-up events; only live events can schedule blocks.
- Large query log tails should not blow up memory in file mode: processing is streaming, not batch-loading into slices.
- Large file tails can still increase one poll cycle duration because the file is processed line by line.
- Pending blocks are checked every second independently from `poll_interval_seconds`.
- `config.json` is reloaded before rules reload and only when the file changes.
- If notification/block thresholds are lowered, existing accumulated profile buckets are evaluated with the new thresholds on the next cycle.
