# telemt: выжимка и план

Контекст для продолжения в новом чате. Ветка `v6-mine`, каталог `services-mine/telemt/` (закоммичен и запушен).

## Задача

MTProto-прокси для Telegram: **секрет на каждого пользователя** и **лимиты на пользователя**
(число конкурентных соединений и устройств). Позже — управление из tgbot.

## Решения

| Вопрос | Решение | Почему |
|---|---|---|
| Реализация | [telemt](https://github.com/telemt/telemt) 3.5.7 | Единственная из проверенных с секретом, лимитами (`max_tcp_conns`, `max_unique_ips`, quota, expiration, rate) и REST API на пользователя. mtg — один секрет, официальный MTProxy — нет лимитов, mtprotoproxy (Python) — нет лимита по IP. |
| Нода | контейнер на **world** | Выход к Telegram с world. |
| Вход | свой домен → **local**, порт 443 держит `https` (caddy layer4). SNI-маршрут на telemt с `proxy-v2` | Порт 443 на local уже занят, PROXY protocol нужен для реальных IP клиентов (иначе `max_unique_ips` не работает). |
| Маскировка | `tls_domain` = свой домен (`TELEMT_DOMAIN`), `mask_host = https`, `mask_port = 444`, caddy отдаёт настоящий сертификат через `SNI_CERT_N` | telemt берёт TLS-параметры и пересылает пробы на `mask_host`, а не на `tls_domain`, поэтому нет петли на себя. Проверено по исходникам `tls_bootstrap.rs`. `SNI_CERT_N` — тот же генератор-native механизм, что у ocserv, без ручных файлов в `config/https/...`. |
| Лимит соединений | `--ips` (устройства, по умолчанию 3) + `--conns` (защита от флуда, по умолчанию 40) | Один клиент Telegram держит несколько TCP-соединений, `--conns` не равен числу устройств. |
| Права на конфиг | обёртка над образом: старт от root, `chown` каталога, `su-exec` в uid 65532 | Оригинальный образ distroless, без оболочки. Entrypoint же создаёт конфиг при первом старте. |
| Управление | `telemt-users.sh` поверх REST API telemt | Тот же API потом использует tgbot. |

## Что уже есть в каталоге

- `docker-compose.yml`, `Dockerfile`, `entrypoint.sh` — сервис и образ-обёртка. Домен задаётся через
  `TELEMT_DOMAIN` (entrypoint подставляет его в `config.toml` вместо `__TLS_DOMAIN__` при первом старте).
- `config.example.toml` — шаблон конфига (порт 8443, `proxy_protocol`, API с токеном, лимиты по умолчанию,
  плейсхолдер `__TLS_DOMAIN__`).
- `telemt-users.sh` — `list`, `info`, `link`, `add`, `set`, `del`, `enable`, `disable`, `rotate`, `reset-quota`.
- `README.md` — установка и команды.

Сертификат для маскировки (`mask_host=https, mask_port=444`) больше не требует ручного файла в
`config/https/config/sites-enabled/` — используется генератор-native `SNI_CERT_N` на сервисе `https`
(тот же механизм, что и для ocserv), см. README.

## Этап 1: запуск и проверка

Сделано (в этом рабочем каталоге):

1. ✅ `services-mine/telemt/` закоммичен и запушен в основной репозиторий (`v6-mine`, `150c623 plan`).
   Все примеры в этих файлах — плейсхолдерный домен/`__TLS_DOMAIN__`, реальный домен нигде в основном
   репозитории не хранится, он только в приватном `config-docker-swarm/`.
2. ✅ В `config-docker-swarm/docker-compose.override.yml` (реальный конфиг для swarm-деплоя, отдельный
   git-репозиторий, целиком в `.gitignore` основного репо — пользователь переносит его на сервер вручную):
   - `https.environment`: `SNI_ROUTE_5=<реальный домен>:telemt.antizapret:8443:proxy-v2` (реальный трафик,
     следующий свободный номер после `SNI_ROUTE_1..4`) и `SNI_CERT_1=<реальный домен>:/data/telemt`
     (ACME-сертификат для маскировки, генератор-native, без ручных файлов);
   - сервис `telemt` (`extends: services-mine/telemt/docker-compose.yml`) с `TELEMT_DOMAIN=<реальный домен>`,
     рядом с `dns-guard`/`caddy-dav`.
   Проверено `docker compose -f docker-compose.yml -f config-docker-swarm/docker-compose.override.yml config`:
   сервис резолвится, `placement: world`, алиасы сети, API только на `127.0.0.1:9091`, volume `config-mine/telemt`.

Осталось — только на серверных нодах (нет доступа из этого рабочего каталога):

3. **local**: после деплоя `https` подхватит `SNI_CERT_1` сам (генератор `init.sh` перечитывает переменные
   при каждом старте контейнера) — отдельного шага/рестарта под это не нужно, только сам деплой из п.5.
   Порт 80 должен быть доступен для ACME HTTP-01.
4. **world**: собрать образ `docker compose build telemt` (или собрать и запушить `nmisha/antizapret-vpn-telemt:3.5.7`).
5. Перенести обновлённый `config-docker-swarm/docker-compose.override.yml` на сервер, задеплоить (`sr_swarm_start.sh`).
6. Проверить:
   - `docker logs` telemt: строки про TLS-fetch и mask без ошибок, конфиг создан;
   - `sudo services-mine/telemt/telemt-users.sh list` и `link admin`;
   - подключение из Telegram по ссылке, в `list` виден клиент и его IP;
   - два устройства с разных IP при `--ips 1`: второе должно быть отклонено.

### Места, где можно ошибиться

- Резолвинг `telemt.antizapret` из контейнера `https` на local через overlay (в compose добавлены алиасы `telemt` и `telemt.antizapret`).
- Пока нет ACME-сертификата на домен, caddy отдаёт запасной сертификат, и эмуляция TLS в telemt возьмёт не те параметры.
  После получения сертификата перезапустить telemt.
- Доступ к API с хоста: из-за NAT docker источник запроса может быть любым адресом в `10.0.0.0/8` или `172.16.0.0/12`. Оба диапазона в `whitelist`.
- `telemt-users.sh` читает токен из `config.toml` (владелец uid 65532, режим 600), поэтому нужен `sudo`.
  Альтернатива: `TELEMT_API_TOKEN` в override и `TELEMT_TOKEN` для скрипта.
- Если telemt после API-правок перезаписывает `config.toml` с другими правами, пересоздать контейнер: entrypoint вернёт владельца.

## Этап 2: интеграция в tgbot

В [services-mine/tgbot/](../tgbot/) уже есть управление профилями wg, ovpn и awg (Go, `internal/bot/`). Добавить по аналогии раздел MTProto:

1. Клиент REST API telemt: `internal/bot/telemt_client.go`. Адрес `http://telemt.antizapret:9091/v1`, заголовок `Authorization: <токен>`.
   Env в compose tgbot: `TELEMT_HOST`, `TELEMT_PORT`, `TELEMT_TOKEN`, таймаут — по образцу `WG_*`, `AWG_*`, `OVPN_*`.
2. Действия: создать пользователя (`POST /v1/users`), список и статистика (`GET /v1/users`: `current_connections`, `active_unique_ips`, `total_octets`),
   выдать ссылку (`links.tls[0]`), изменить лимиты (`PATCH /v1/users/{name}`), отключить и включить, сменить секрет (`rotate-secret`), удалить.
3. Связка с пользователями бота: имя пользователя telemt = логин из `users_store` / `accounts_store`, роли и права как у wg/ovpn.
4. Обработчики и callbacks по образцу `callbacks_wg*.go`, `handlers_wgprofiles.go`, `wg_pending_handler.go`; регистрация в `router.go`, `command_registry.go`, `help.go`.
5. Сетевая доступность: tgbot стоит на world (`node.labels.location == world`), как и telemt. Если ноды разойдутся, порт API придётся открыть только внутри overlay.
6. Тесты клиента на моке API (как `wg_filename_test.go`).

Справочник API: <https://github.com/telemt/telemt/blob/main/docs/Architecture/API/API.md>
(схемы `CreateUserRequest`, `PatchUserRequest`, `UserInfo`, `UserLinks`).

## Открытые вопросы

- Значения по умолчанию лимитов (3 IP, 40 соединений) — подобрать на практике.
- `SNI_CERT_N`-сайт отдаёт `respond 204` (как у ocserv) — этого может быть достаточно для маскировки,
  но если понадобится более правдоподобная страница, единственный способ — ручной файл в
  `config/https/config/sites-enabled/` (сайт `import`-ится и не участвует в регенерации), тогда это
  вытеснит `SNI_CERT_N` для этого домена.
- Нужен ли CI-билд образа (`versions.json`), если образ будет собираться руками на world-ноде.
