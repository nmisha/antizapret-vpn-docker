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
