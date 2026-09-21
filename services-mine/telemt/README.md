# telemt — MTProto proxy с секретом и лимитами на пользователя

[telemt](https://github.com/telemt/telemt) (Rust), fake-TLS. У каждого пользователя свой секрет и своя ссылка `tg://proxy`,
лимиты на пользователя: уникальные IP (устройства), TCP-соединения, срок, трафик, скорость.
Изменения применяются на лету, рестарт не нужен.

## Схема

```text
Telegram-клиент ──443, SNI=<ваш домен>──▶ https (local, caddy layer4)
                                            │ proxy-v2, overlay
                                            ▼
                             telemt (world), порт 8443 ──▶ серверы Telegram (выход с world)
   пробы без секрета / TLS-fetch ◀── https:444 (caddy, настоящий сертификат на домен, SNI_CERT_N)
```

- Домен указывает на local. Порт 443 на нём уже держит `https`, telemt наружу порты не публикует.
- PROXY protocol v2 передаёт telemt реальный IP клиента, без него `--ips` не работал бы (telemt видел бы только IP контейнера `https`).
- `tls_domain` = ваш домен (`TELEMT_DOMAIN`). Сертификат для него отдаёт caddy через `SNI_CERT_N`
  (тот же генератор, что для ocserv/vhost-доменов), поэтому пробы и эмуляция TLS видят обычный сайт —
  никаких файлов вручную в `config/https/...` копировать не нужно, всё из переменных окружения.

## Установка

Везде ниже `<домен>` — один и тот же домен (например `tp.example.com`), указывающий на local.

1. **local**: в `docker-compose.override.yml` у сервиса `https` — маршрут для реального трафика
   (номер `N` в `SNI_ROUTE_N` — следующий свободный, нумерация без пропусков) и сертификат для
   маскировки (номер в `SNI_CERT_N` — свой отдельный, тоже следующий свободный):

   ```yaml
   https:
     environment:
       - SNI_ROUTE_N=<домен>:telemt.antizapret:8443:proxy-v2
       - SNI_CERT_N=<домен>:/data/telemt
   ```

   `SNI_CERT_N` — тот же механизм, что использует ocserv для своего сертификата: caddy сам получает
   ACME-сертификат на `<домен>` и поднимает для него минимальный сайт (`respond 204`) на порту
   `PROXY_HTTPS_PORT` (по умолчанию 444) — это и есть `mask_host=https, mask_port=444` в конфиге telemt.
   `/data/telemt` — каталог экспорта файлов сертификата внутри `https`, telemt их не читает
   (сверяет TLS живым подключением), но каталог обязателен и должен быть уникальным.
2. **world**: собрать образ (нужен на той ноде, где запустится сервис; `build` в swarm не работает):

   ```sh
   docker compose build telemt          # локально на world-ноде
   # или собрать где угодно и docker push nmisha/antizapret-vpn-telemt:3.5.7
   ```

3. В `docker-compose.override.yml`:

   ```yaml
   telemt:
     extends:
       file: services-mine/telemt/docker-compose.yml
       service: telemt
     environment:
       - TELEMT_DOMAIN=<домен>
   ```

4. Деплой (`sr_swarm_start.sh`). При первом старте entrypoint сам создаёт `config-mine/telemt/config.toml`
   (случайный токен API, пользователь `admin`, домен из `TELEMT_DOMAIN`), выставляет владельца каталога
   и понижает права до uid 65532. Ручной `chown` не нужен.
5. На world-ноде запустить `telemt-users.sh link admin` в сетевом namespace контейнера (см. раздел API).

Одиночный сервер (без world): `https` и telemt на одной машине, схема та же, порт 443 по-прежнему занят `https`.

## Пользователи

```sh
t=services-mine/telemt/telemt-users.sh
sudo $t add vasya --ips 2 --conns 30           # создать, выводит ссылку
sudo $t set vasya --ips 3 --expires 2026-12-31T23:59:59Z
sudo $t set vasya --ips none                   # вернуть значение по умолчанию
sudo $t list                                   # соединения/лимит, IP/лимит, трафик
sudo $t disable vasya                          # блокирует и рвёт активные сессии
sudo $t rotate vasya                           # новый секрет, старая ссылка перестаёт работать
sudo $t del vasya
```

Лимиты: `--ips --conns --expires --quota --up --down`; `none` снимает индивидуальный лимит. Скрипту нужен токен API:
он берётся из `TELEMT_TOKEN` или из `auth_header` в `config.toml` (файл читается root/uid 65532, поэтому `sudo`).
Можно зафиксировать токен переменной `TELEMT_API_TOKEN` в override и передавать его скрипту как `TELEMT_TOKEN`.

## Про лимиты соединений

Один клиент Telegram держит несколько TCP-соединений, поэтому `--conns` — это не число устройств.

- **Устройства** — `--ips` (уникальные IP-источники). По умолчанию 3 (`user_max_unique_ips_global_each`).
- **`--conns`** — защита от флуда и раздачи ссылки; по умолчанию 40 (`user_max_tcp_conns_global_each`).
  Значения меньше ~8 на устройство приведут к обрывам.

Значения по умолчанию лежат в секции `[access]` конфига. Их можно править прямо в файле, telemt подхватит изменения сам (hot reload).

## Лимиты через Compose override

В `services.telemt.environment` файла `config-docker-swarm/docker-compose.override.yml`:

```yaml
- TELEMT_MAX_TCP_CONNS=40
- TELEMT_MAX_UNIQUE_IPS=3
```

Это значения по умолчанию **на каждого пользователя**, а не на весь сервер.
Entrypoint при каждом запуске записывает их в `[access]` существующего или нового
`config.toml`: `user_max_tcp_conns_global_each` и `user_max_unique_ips_global_each`.
Пустая переменная сохраняет значение в файле; принимаются положительные целые числа.
Индивидуальные лимиты пользователей, заданные через API/панель, сохраняются
и имеют приоритет. Изменения общих значений через панель будут заменены значениями
Compose при следующем запуске, если переменные непустые.

TCP-соединения не равны устройствам: один клиент открывает несколько соединений.
IP также не равен устройству: несколько устройств за NAT имеют один адрес.
Режим учёта IP задаёт `user_max_unique_ips_mode` в TOML; шаблон использует `active_window`.
После обновления entrypoint необходимо пересобрать образ и применить стек.

## API

`http://telemt.antizapret:9091/v1/...` в docker-сети (заголовок `Authorization: <auth_header>`).
На localhost world-хоста API не опубликован. Для CLI на world-ноде из корня проекта:

```sh
cid=$(docker ps -q --filter label=com.docker.swarm.service.name=antizapret_telemt)
test -n "$cid" || { echo 'telemt container is not running on this node'; exit 1; }
pid=$(docker inspect --format '{{.State.Pid}}' "$cid")
sudo nsenter -t "$pid" -n bash services-mine/telemt/telemt-users.sh list
sudo nsenter -t "$pid" -n bash services-mine/telemt/telemt-users.sh link admin
```

Нужны `nsenter`, `bash`, `curl`, `jq` и `column` на world-хосте.
`nsenter -n` меняет только сеть; скрипт и config.toml читаются с хоста.
Порт наружу не публикуется. Документация:
<https://github.com/telemt/telemt/blob/main/docs/Architecture/API/API.md>. Следующий этап — раздел в tgbot поверх этого API
(`POST/PATCH/DELETE /v1/users`, ссылки в `links.tls`).

## Не проверено на реальных нодах

Локально проверены сборка образа 3.5.7, генерация конфига, запуск с read-only и ограниченными capabilities,
healthcheck и преобразование полного Compose в Swarm. PROXY protocol между `https` и telemt,
получение сертификата для домена в caddy и подключение клиента требуют проверки на сервере: `docker logs`
сервиса telemt (строки про TLS-fetch и mask), `telemt-users.sh list`, подключение по ссылке.

## Спонсорский канал и подключение к Telegram

Спонсорский канал — способ продвигать публичный Telegram-канал среди пользователей
прокси. Это может быть свой канал или платное размещение чужого по договорённости
с владельцем. Само подключение `ad_tag` не даёт автоматических выплат от Telegram,
не подписывает пользователей на канал и не улучшает скорость, стабильность,
обход блокировок или поддержку звонков. Для личного прокси польза обычно невелика.

Для настройки:

1. В [@MTProxybot](https://t.me/MTProxybot) отправьте `/newproxy`, публичный IP
   входного сервера с Caddy и порт `443` (не внутренний порт telemt `8443`).
2. Отправьте боту базовый секрет выбранного пользователя из `[access.users]`:
   32 hex-символа, без префикса `ee` и домена Fake TLS.
3. Полученный тег добавьте в существующую секцию `[general]` файла
   `config-mine/telemt/config.toml` на узле с telemt; не дублируйте секцию:

   ```toml
   [general]
   use_middle_proxy = true
   ad_tag = "0123456789abcdef0123456789abcdef" # заменить тегом от бота
   ```

4. На Swarm manager выполните `docker service update --force antizapret_telemt`.
5. В боте: `/myproxies` → нужный прокси → `Set promotion` → публичная ссылка
   на канал. По документации telemt, обновление может занять около часа.

Приватные каналы не поддерживаются. Если пользователь уже подписан на канал,
спонсорское размещение ему не показывается. Для подключения оставьте свою
Fake TLS-ссылку: ссылку, выданную ботом, использовать не следует.
Реальные секреты и теги не добавляйте в публичные примеры конфигурации.

Разным пользователям можно задать разные теги; индивидуальные значения имеют
приоритет над общим `general.ad_tag`:

```toml
[access.user_ad_tags]
alice = "0123456789abcdef0123456789abcdef"
bob = "abcdef0123456789abcdef0123456789"
```

### Куда идёт трафик

telemt — программа на вашем сервере. Для передачи сообщений она подключается
к инфраструктуре Telegram, а не к собственным серверам разработчиков telemt.
Два транспортных режима:

- `use_middle_proxy = true`: telemt → Middle-End (ME) Telegram → дата-центры
  Telegram. ME — промежуточные серверы самого Telegram; этот режим нужен для
  спонсорских каналов.
- `use_middle_proxy = false`: telemt → дата-центры Telegram напрямую (Direct).

В этой схеме развёртывания входящий путь: клиент Telegram → ваш Caddy
(маршрутизация по SNI) → ваш telemt. При последней проверке использовался ME,
а исходящие соединения шли напрямую с узла telemt, без дополнительного SOCKS/VPN.
Если включён `me2dc_fallback`, при недоступности ME возможен переход на Direct.
Режим ME и способ выхода в сеть — разные настройки: отдельно можно настроить
upstream-прокси. Показ спонсорского канала в Direct-режиме не обеспечивается.

Источники: [FAQ telemt](https://github.com/telemt/telemt/blob/main/docs/FAQ.ru.md),
[ME и upstream](https://github.com/telemt/telemt/blob/main/docs/Advanced_settings/TUNING.ru.md).

## Swarm tmpfs and resource limits

Mount `/run/telemt` using `volumes: type: tmpfs`, limited to 4 MiB.
The short service-level `tmpfs:` syntax does not create a mount in stack deploy.
The panel similarly mounts `/tmp` with a 64 MiB limit. Do not assume tmpfs
permissions are 1777: Swarm can preserve 0755 root:root from the image directory.
The telemt entrypoint assigns `/run/telemt` to UID/GID 65532 before dropping
privileges, allowing runtime files to be written with a read-only root filesystem.
Rebuild and publish the telemt image after updating this entrypoint, then redeploy.

Deploy with the root `sr_swarm_start.sh`. After compose2swarm, it converts quoted
numeric size fields to integers required by the Swarm schema. This works around
the serializer without rebuilding the published xtrime/antizapret-vpn:6 converter
image. The old direct pipeline without size normalization is insufficient.
The script validates the stack and stops before deployment if any stage fails.

Memory limits remain 300 MiB for telemt and 256 MiB for the panel; CPU is uncapped.
Used tmpfs memory also counts toward the container limit. On 2026-09-21 the
current tasks peaked at approximately 28 and 22 MiB with no OOM events, so the
observed workload does not justify raising limits. Recheck docker stats and OOM
counters as load grows; these measurements are not a maximum-user capacity test.
