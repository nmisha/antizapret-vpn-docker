# Stalwart + Bulwark

Внешние адреса используют HTTPS на **443**:

- `https://webmail.example.com/` — Bulwark, веб-почта.
- `https://mail.example.com/admin` — управление Stalwart.
- `https://mail.example.com/` — также JMAP/OAuth/WebDAV Stalwart.

Нужны два разных поддомена, отличных от основного домена Dashboard и Authelia.
Их A/AAAA-записи должны вести на сервер HTTPS. Порты 80 (ACME HTTP-01) и 443
должны быть доступны. Порты 10443/10444 больше не используются.

## Настройки в Compose override

Примеры значений находятся в [docker-compose.yml](docker-compose.yml) и
корневом `docker-compose.override.sample.yml`. Отдельный env-файл не нужен.
Задавайте значения непосредственно в `environment` соответствующего сервиса:

```yaml
services:
  stalwart:
    environment:
      - STALWART_HOSTNAME=mail.example.com
      - STALWART_PUBLIC_URL=https://mail.example.com
      - STALWART_RECOVERY_ADMIN=
  webmail:
    environment:
      - JMAP_SERVER_URL=https://mail.example.com
      # Заполните результатом openssl rand -hex 32.
      - SESSION_SECRET=
```

Те же домены укажите в `SNI_ROUTE_N`, `SNI_CERT_N`, `PROXY_VHOST_N` сервиса
`https`, ссылках Dashboard и Homepage. Секрет сохраняйте между перезапусками.
В локальном Swarm override уже указаны `mail.marina.2bd.net` для Stalwart и
`webmail.marina.2bd.net` для Bulwark, а также сгенерированный сессионный секрет.

Для существующей установки проверьте сохранённый `defaultHostname` в Stalwart:
изменение переменных контейнера не заменяет сохранённые настройки сервера.

## Swarm и HTTPS

Используйте настройки из `config-docker-swarm/docker-compose.override.yml` в
эффективном override. Стандартный `compose.swarm.env` выбирает **корневой**
`docker-compose.override.yml`; файл из `config-docker-swarm` автоматически
не подключается. Перенесите его на сервер как корневой override либо укажите
этот путь в `COMPOSE_FILE`. Каталог `config-docker-swarm` исключён из Git.

Два mail-сайта создаются через `PROXY_VHOST_N`, SNI направляет их с 443 на
внутренний TLS-порт Caddy 444. `PROXY_VHOST_AUTH_N=false` оставляет авторизацию
самим приложениям: перенаправление на Authelia ломало бы JMAP/OAuth.
Остальные сайты сохраняют прежнюю защиту Authelia.

Обновлённые `init.sh` и `entrypoint.sh` включены в исходники HTTPS-образа.
Перед развёртыванием пересоберите образ, опубликуйте его под новым тегом и
укажите этот тег в `services.https.image` вашего override. Swarm не собирает
образы при `docker stack deploy`; скрипты запускаются штатным entrypoint образа.

Почтовые порты 25, 465, 587, 993 и 4190 в Swarm публикуются с `mode: host`:
это совместимо с `endpoint_mode: dnsrr` и сохраняет IP клиента без routing mesh.
DNS должен вести на узел, где запущен Stalwart (`node.labels.location == local`).
Для bind mounts нужен один подходящий узел либо отдельная привязка к узлу с данными.

## Первоначальная настройка

1. На узле Stalwart подготовьте каталоги `config-mine/mail/etc` и
   `config-mine/mail/data` с владельцем UID 2000. Не меняйте права существующих
   данных без проверки текущего образа и владельца.
2. Откройте `https://mail.example.com/admin`, возьмите временный пароль из логов
   Stalwart и завершите мастер. `STALWART_RECOVERY_ADMIN` можно задать для
   фиксированных bootstrap/recovery credentials; после настройки удалите его.
3. Сохраните внутренний HTTP listener на `0.0.0.0:8080` для reverse proxy,
   включая после завершения bootstrap. Он не публикуется на хосте.
   Настройте доверие к forwarded-заголовкам только от HTTPS-прокси.
4. Для SMTP/IMAP настройте TLS в Stalwart. Caddy экспортирует сертификат в
   `/certs/mail/certificate.crt` и ключ в `/certs/mail/certificate.key` внутри
   Stalwart. `SNI_CERT_UID_N=2000` обеспечивает доступ к ключу с правами 0600.
   Само монтирование файлов не подключает их к listeners: настройте сертификат
   и механизм его перечитывания при обновлении в используемой версии Stalwart.
5. Создайте почтовый домен, пользователей и DNS-записи MX/SPF/DKIM/DMARC/PTR.
   Затем проверьте вход в Bulwark и отправку/приём почты. Bulwark использует
   публичный JMAP URL, доступный также из контейнеров.

Образы сейчас используют `latest`; перед развёртыванием существующей установки
сверьте версию и закрепите подходящий тег. Изменение переменной окружения не
заменяет миграцию сохранённой конфигурации между версиями Stalwart.

Документация: [Stalwart Docker](https://stalw.art/docs/install/platform/docker/),
[переменные Stalwart](https://stalw.art/docs/configuration/environment-variables/),
[Bulwark Compose](https://bulwarkmail.org/docs/deployment/docker/compose).
