# PR в xtrime-ru/antizapret-vpn-docker — черновики

Ветки запушены в `nmisha/antizapret-vpn-docker`. PR 1–5: база `upstream/v6` (3c15feb), каждая сливается без конфликтов
и в `v6`, и в `dev`. Порядок открытия: от меньшего к большему.

Как открыть: перейти по ссылке → GitHub покажет форму «Open a pull request» (base: `xtrime-ru/v6`,
compare: `nmisha:pr/...`) → вставить Title и Description → Create pull request.
Если автор предпочитает `dev` — сменить base в форме на `dev`.

---

## 1. WireGuard: не затирать host

Ссылка: https://github.com/xtrime-ru/antizapret-vpn-docker/compare/v6...nmisha:antizapret-vpn-docker:pr/wireguard-keep-host?expand=1

**Title:** `wireguard: keep stored host when public IP detection fails`

**Description:**
```markdown
## Problem
When `WG_HOST` is not set, `init.sh` detects the public IP with `timeout 1s curl -4 icanhazip.com` and does not validate the result. If the request is slow or fails, `update_db` writes `host=''` into `user_configs_table` on every container start, so newly created client profiles have no `Endpoint`.

## Changes
- Detect the IP with `--connect-timeout 3 --max-time 5` and accept only an IPv4 address; log a warning otherwise.
- Update `user_configs_table.host` only when a value is available, keeping the previously stored host.

## Testing
`bash -n services/wireguard/init.sh`. Behaviour change is limited to the empty-host case.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## 2. dnsmap: отдельный поток для iptables

Ссылка: https://github.com/xtrime-ru/antizapret-vpn-docker/compare/v6...nmisha:antizapret-vpn-docker:pr/dnsmap-mapping-worker?expand=1

**Title:** `dnsmap: apply iptables batches in a dedicated worker thread`

**Description:**
```markdown
## Problem
In `add_mappings` the first request that finds no running batch becomes the batch leader and calls `apply_mapping_batches()` in its own DNS handler thread. The leader keeps applying every following batch, so its own reply waits for later clients. If anything other than `OSError` is raised there, `mapping_batch_running` stays `True` and later requests wait forever. `iptables-restore` also has no timeout.

## Changes
- Batches are applied by a daemon worker thread; each request waits only for its own mapping.
- If the worker fails unexpectedly, pending waiters are released and their fake addresses are returned to the pool, so the next request can retry.
- `iptables-restore` runs with `timeout=10`; `TimeoutExpired` is handled like `OSError`.

## Testing
`python3 -B -m unittest test_dnsmap` (46 tests, 3 new):
- first request returns while the next batch is still blocked
- `OSError`, `TimeoutExpired` and an unexpected exception release waiters and allow a retry
- `close()` drains a pending batch without waiting for the batch delay

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## 3. CoreDNS: ждать Docker DNS вместо фиктивных адресов

Ссылка: https://github.com/xtrime-ru/antizapret-vpn-docker/compare/v6...nmisha:antizapret-vpn-docker:pr/coredns-discovery?expand=1

**Title:** `coredns: wait for Docker DNS instead of forwarding to placeholder IPs`

**Description:**
```markdown
## Problem
`config.sh` falls back to `169.254.0.1` / `169.254.0.3` when `az-local` or `adguard` is not yet registered in Docker DNS. CoreDNS then starts (or reloads) with unreachable upstreams until a later healthcheck, and a short discovery outage replaces a working Corefile with placeholders.

## Changes
- Resolve IPv4 only (`getent ahostsv4`) with a 3s timeout.
- Without `az-local`/`adguard` addresses keep the existing Corefile; fail only when there is none yet.
- Write the Corefile to a temporary file and rename it, so the `reload` plugin never reads a partially written file.
- The entrypoint waits for the first successful config before starting CoreDNS.
- The healthcheck treats "no Corefile yet" as startup, so the `|| kill -TERM 1` healthcheck does not restart a container that is still waiting for Docker DNS.

Not included: forward `max_fails` / `health_check` tuning — that is a separate trade-off.

## Testing
Scripts run in `ubuntu:24.04` with a mocked `getent`:
| Case | Result |
|---|---|
| healthcheck before the first Corefile | exit 0 |
| adguard missing, no Corefile | exit 1 (entrypoint keeps waiting) |
| local + adguard | `forward . <local> <adguard>` |
| local + world + adguard | `forward . <world> <local> <adguard>` |
| discovery outage with existing Corefile | exit 0, Corefile unchanged |
| no temporary files left | ✔ |

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## 4. API: один grep на запрос

Ссылка: https://github.com/xtrime-ru/antizapret-vpn-docker/compare/v6...nmisha:antizapret-vpn-docker:pr/api-grep-per-request?expand=1

**Title:** `api: run one grep per /list/ request instead of a shared filter process`

**Description:**
```markdown
## Problem
The exclude filter is a long-lived `grep` per list, fed in batches of ~1000 lines with the literal line `__DELIM__` marking the end of a batch:

- An exclude regex that matches `__DELIM__` (for example `[A-Z]`, `_`, `.*`) removes the delimiter. The reader then waits forever while holding the filter locks, and every `/list/` and `/update/` request hangs.
- The whole batch is written before any output is read. With long lines grep's output pipe fills, grep stops reading stdin and both sides block.
- `adaptList` takes `RLock` twice on the same `RWMutex`; with `/update/` waiting for `Lock` this can deadlock.
- If grep dies, the dead filter stays installed until the next `/update/`, and lists are answered with `200` and a truncated body, which AdGuard accepts as complete.
- `sed -i` / `gawk -i inplace` rewrite the user's exclude files on every reload.

## Changes
- Each request starts one `grep -a -v -E` for the combined dist + custom patterns, passed over a pipe as `/dev/fd/3`. Input is streamed from a goroutine while output is read; end of input is stdin EOF. No delimiter, no shared process, no locks between requests.
- Filters are immutable in memory and swapped atomically (`atomic.Pointer`); a failed `/update/` keeps the previous filters.
- Patterns are validated with `grep -E` on load; invalid ones are skipped with a warning instead of breaking the filter. Blank lines and full-line comments are ignored (an empty pattern would match everything). `/regex/` lines are unwrapped. GNU grep semantics are kept.
- A missing filter returns `503` before any output. An error after streaming started aborts the connection (`http.ErrAbortHandler`), so clients see an incomplete download instead of a short list.
- `-a` keeps lines with invalid UTF-8 as text instead of "binary file matches".
- Exclude files are no longer modified.

## Performance
~420 patterns from `exclude-hosts-dist.txt`, 300k domains, full `/list/?filter_dist=1&filter_custom=1` request, identical output:

| Implementation | Time |
|---|---|
| persistent grep, 1000-line batches (pipe I/O already concurrent) | ~310 ms |
| one grep per request | ~75 ms |

(A pure Go `regexp` version was also tried: ~46 s for the same input, because RE2 has no multi-pattern literal prefilter.)

## Testing
`go vet` + `go test -race` on Linux (`golang:1.26`, same as the Dockerfile build stage): 18 tests pass, including patterns matching the old delimiter, blank lines, invalid patterns, excluding everything, 8 concurrent requests, aborted connection on a truncated source, 503 without a filter, and keeping filters after a failed update.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## 5. routes: сверка с ядром и устойчивость списков

Ссылка: https://github.com/xtrime-ru/antizapret-vpn-docker/compare/v6...nmisha:antizapret-vpn-docker:pr/routes-reconcile?expand=1

**Title:** `routes: reconcile with kernel tables and tolerate bad route list lines`

**Description:**
```markdown
## Problem
- Routes are (re)installed only when the resolved gateway IP changes. A route deleted or changed outside the process (network reattach, `ip route flush`) is never restored.
- Interface indexes are cached forever, but they can change on network reattach while the gateway IP stays the same.
- One invalid line in a route list (for example an IPv6 network in `include-ips-custom.txt`) rejects the whole list, and the first failing route stops the rest of the list.
- VPN route lists are requested without `filter_custom=0`, so the IP lists pass through `exclude-hosts-custom.txt`, which contains host regexes. A pattern like `\.1` or `^10` can silently drop a route while clients still get the network in AllowedIPs.

## Changes
- Each cycle reads the main and VPN (100) tables once and replaces only routes that differ from the desired state; interface indexes are resolved again every cycle.
- The last successfully fetched VPN route list is kept, so deleted routes are repaired even while the list API is unavailable.
- Invalid list lines are logged and skipped; a response without any valid IPv4 route is still rejected.
- A failing route no longer stops the remaining routes; errors are collected and returned.
- IPv6 CIDRs are rejected as destinations instead of failing in netlink.
- Route list URLs pass `filter_custom=0`. IP exclusions are still applied by `parse.sh` via `exclude-ips*-custom.txt`.

## Testing
`go vet` + `go test -race` on Linux: 20 tests pass, including repair after deletion and interface change, snapshot errors not writing routes, reusing the cached list without HTTP, skipping invalid lines and continuing after an error, and `filter_custom=0` in list URLs.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## 6. doall: flock вместо блокировки по существованию файла — **в `dev`**

Ветка собрана от `upstream/dev` (1356764) и содержит один коммит поверх него. Base в форме — **`dev`**:
PR в `v6` протащил бы за собой 3 ещё не влитых коммита dev.

Ссылка: https://github.com/xtrime-ru/antizapret-vpn-docker/compare/dev...nmisha:antizapret-vpn-docker:pr/doall-flock?expand=1

**Title:** `doall: use flock instead of an existence-based lock file`

**Description:**
```markdown
## Problem
`doall.sh` waits while `/dev/shm/.doall_lock` exists, then creates it and removes it in a trap. Two failure modes:

- **Race.** The healthcheck and the 6h refresh loop can both see no file and run download/parse concurrently. With three parallel runs the new test observes `start,start,start,end,end,end` on every run.
- **Stale lock.** doall runs under `timeout --kill-after=5s`. After `SIGKILL` no trap runs, the file stays, and every later doall waits forever (each until its own timeout). Moving the file to `/dev/shm` clears it only on a container restart.

## Changes
- Lock the open file with `flock(1)` (`exec 9>/dev/shm/.doall_lock; flock 9`). The kernel releases the lock when the holder dies, even by `SIGKILL`, and taking it is atomic.
- The file is never unlinked, so every waiter uses the same inode; `init.sh` no longer removes it. `/dev/shm` is still cleared on restart.
- Children inherit fd 9, so a killed doall keeps the lock until its download/parse child exits: refreshes never overlap.

`flock` is part of `util-linux`, which is essential in `ubuntu:24.04`; no new packages.

## Testing
New `test_doall_lock.py`, run in the Dockerfile test stage (`docker build --target test-dnsmap` passes):

| Test | Current `dev` | This PR |
|---|---|---|
| three parallel refreshes are serialized | FAIL (`start,start,start,end,end,end`) | OK |
| a leftover lock file does not block | ERROR (waits forever) | OK |
| a SIGKILLed doall releases the lock once its child exits | ERROR (waits forever) | OK |

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## Отложено

- **Исключения и валидация списков** (sanitize без обрезки ASN, точное сравнение CIDR, пропуск невалидных IP) —
  построено поверх staging-логики `parse.sh`, которой в upstream нет. Делать после PR «Списки: flock/staging/.ready».
- Спорные (сначала issue): CoreDNS `max_fails`/`health_check`, серверный `az_clients`, https SNI/Authelia.
