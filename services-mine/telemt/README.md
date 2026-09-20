# telemt — MTProto proxy с секретом и лимитами на пользователя

[telemt](https://github.com/telemt/telemt) (Rust), fake-TLS. У каждого пользователя свой секрет и своя ссылка `tg://proxy`,
лимиты на пользователя: уникальные IP (устройства), TCP-соединения, срок, трафик, скорость.
Изменения применяются на лету, рестарт не нужен.

## Схема

```text
Telegram-клиент ──443, SNI=tp.sl.vmvs.work.gd──▶ https (local, caddy layer4)
                                                   │ proxy-v2, overlay
                                                   ▼
                                    telemt (world), порт 8443 ──▶ серверы Telegram (выход с world)
   пробы без секрета / TLS-fetch ◀── https:444 (caddy, настоящий сертификат на домен)
```

- Домен `tp.sl.vmvs.work.gd` указывает на local. Порт 443 на нём уже держит `https`, telemt наружу порты не публикует.
- PROXY protocol v2 передаёт telemt реальный IP клиента, без него `--ips` не работал бы (telemt видел бы только IP контейнера `https`).
- `tls_domain` = ваш домен. Сертификат для него отдаёт caddy, поэтому пробы и эмуляция TLS видят обычный сайт.

## Установка

1. **local**: маршрут в `docker-compose.override.yml` (номер `N` — следующий свободный, нумерация без пропусков):

   ```yaml
   https:
     environment:
       - SNI_ROUTE_N=tp.sl.vmvs.work.gd:telemt.antizapret:8443:proxy-v2
   ```

2. **local**: сайт для сертификата — скопировать [telemt.caddy.example](telemt.caddy.example) в
   `config/https/config/sites-enabled/telemt.caddy`, перезапустить `https`
   (`docker service update --force antizapret_https || docker compose restart https`).
3. **world**: собрать образ (нужен на той ноде, где запустится сервис; `build` в swarm не работает):

   ```sh
   docker compose build telemt          # локально на world-ноде
   # или собрать где угодно и docker push nmisha/antizapret-vpn-telemt:3.5.7
   ```

4. В `docker-compose.override.yml`:

   ```yaml
   telemt:
     extends:
       file: services-mine/telemt/docker-compose.yml
       service: telemt
   ```

5. Деплой (`sr_swarm_start.sh`). При первом старте entrypoint сам создаёт `config-mine/telemt/config.toml`
   (случайный токен API и пользователь `admin`), выставляет владельца каталога и понижает права до uid 65532.
   Ручной `chown` не нужен.
6. На world-ноде: `sudo services-mine/telemt/telemt-users.sh link admin` — ссылка для Telegram.

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

`http://127.0.0.1:9091/v1/...` на world-хосте (заголовок `Authorization: <auth_header>`), в docker-сети — `telemt.antizapret:9091`.
Порт наружу не публикуется. Документация:
<https://github.com/telemt/telemt/blob/main/docs/Architecture/API/API.md>. Следующий этап — раздел в tgbot поверх этого API
(`POST/PATCH/DELETE /v1/users`, ссылки в `links.tls`).

## Не проверено на реальных нодах

Скрипт проверен на моке API, логика entrypoint — локально без docker. Сборка образа, PROXY protocol между `https` и telemt,
получение сертификата для домена в caddy и подключение клиента не запускались. Первый запуск стоит проверить: `docker logs`
сервиса telemt (строки про TLS-fetch и mask), `telemt-users.sh list`, подключение по ссылке.
