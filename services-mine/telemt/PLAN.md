# telemt: выжимка и план

Контекст для продолжения в новом чате. Ветка `v6-mine`, каталог `services-mine/telemt/` (в git пока не добавлен).

## Задача

MTProto-прокси для Telegram: **секрет на каждого пользователя** и **лимиты на пользователя**
(число конкурентных соединений и устройств). Позже — управление из tgbot.

## Решения

| Вопрос | Решение | Почему |
|---|---|---|
| Реализация | [telemt](https://github.com/telemt/telemt) 3.5.7 | Единственная из проверенных с секретом, лимитами (`max_tcp_conns`, `max_unique_ips`, quota, expiration, rate) и REST API на пользователя. mtg — один секрет, официальный MTProxy — нет лимитов, mtprotoproxy (Python) — нет лимита по IP. |
| Нода | контейнер на **world** | Выход к Telegram с world. |
| Вход | домен `tp.sl.vmvs.work.gd` → **local**, порт 443 держит `https` (caddy layer4). SNI-маршрут на telemt с `proxy-v2` | Порт 443 на local уже занят, PROXY protocol нужен для реальных IP клиентов (иначе `max_unique_ips` не работает). |
| Маскировка | `tls_domain` = свой домен, `mask_host = https`, `mask_port = 444`, caddy отдаёт настоящий сертификат | telemt берёт TLS-параметры и пересылает пробы на `mask_host`, а не на `tls_domain`, поэтому нет петли на себя. Проверено по исходникам `tls_bootstrap.rs`. |
| Лимит соединений | `--ips` (устройства, по умолчанию 3) + `--conns` (защита от флуда, по умолчанию 40) | Один клиент Telegram держит несколько TCP-соединений, `--conns` не равен числу устройств. |
| Права на конфиг | обёртка над образом: старт от root, `chown` каталога, `su-exec` в uid 65532 | Оригинальный образ distroless, без оболочки. Entrypoint же создаёт конфиг при первом старте. |
| Управление | `telemt-users.sh` поверх REST API telemt | Тот же API потом использует tgbot. |

## Что уже есть в каталоге

- `docker-compose.yml`, `Dockerfile`, `entrypoint.sh` — сервис и образ-обёртка.
- `config.example.toml` — шаблон конфига (порт 8443, `proxy_protocol`, API с токеном, лимиты по умолчанию).
- `telemt-users.sh` — `list`, `info`, `link`, `add`, `set`, `del`, `enable`, `disable`, `rotate`, `reset-quota`.
- `telemt.caddy.example` — сайт caddy для сертификата на домен.
- `README.md` — установка и команды.

## Этап 1: запуск и проверка (не сделано, ничего не запускалось на реальных нодах)

1. Закоммитить и запушить `services-mine/telemt/`, подтянуть на нодах. `config-mine/` и `config/` в git не входят.
2. **local**: добавить в override `https` переменную `SNI_ROUTE_N=tp.sl.vmvs.work.gd:telemt.antizapret:8443:proxy-v2`
   (N — следующий свободный номер, без пропусков).
3. **local**: скопировать `telemt.caddy.example` в `config/https/config/sites-enabled/telemt.caddy`, перезапустить `https`.
   Порт 80 должен быть доступен для ACME HTTP-01.
4. **world**: собрать образ `docker compose build telemt` (или собрать и запушить `nmisha/antizapret-vpn-telemt:3.5.7`).
5. Добавить сервис `telemt` в `docker-compose.override.yml` (`extends` на `services-mine/telemt/docker-compose.yml`), задеплоить (`sr_swarm_start.sh`).
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
- Что отдавать на сайте `tp.sl.vmvs.work.gd` вместо `respond "OK"` — статическую страницу правдоподобнее.
- Нужен ли CI-билд образа (`versions.json`), если образ будет собираться руками на world-ноде.
