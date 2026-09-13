# 2FAuth

Сервис использует официальный образ `2fauth/2fauth`, SQLite и каталог
`config-mine/2fauth` для постоянного хранения данных. Внутри Docker доступен
по адресу `http://2fauth:8000`; порты на хосте не публикуются.

Добавьте в секцию `services` корневого `docker-compose.override.yml`:

```yaml
  2fauth:
    extends:
      file: services-mine/2fauth/docker-compose.yml
      service: 2fauth
```

Подготовьте каталог на Linux-хосте из корня проекта:

```sh
mkdir -p config-mine/2fauth
sudo chown 1000:1000 config-mine/2fauth
sudo chmod 700 config-mine/2fauth
```

Сгенерируйте ключ локально:

```sh
docker run --rm --entrypoint /usr/bin/php 2fauth/2fauth:latest artisan key:generate --show
```

В корневом `.env` задайте `TWOFAUTH_APP_KEY` равным полученному ключу, а
`TWOFAUTH_APP_URL` — фактическому внешнему HTTPS-адресу, включая порт, если
он нестандартный. Сохраните ключ вместе с резервной копией данных и не
генерируйте его заново при перезапуске или обновлении.

Для доступа через существующий сервис `https` можно добавить в его настройки
в `docker-compose.override.yml` следующий маршрут (номер `9` и порт `6443`
должны быть свободны; иначе выберите другие):

```yaml
  https:
    ports:
      - "6443:6443"
    environment:
      - PROXY_SERVICE_9=2FAuth:6443:2fauth:8000
```

В этом примере `TWOFAUTH_APP_URL=https://<ваш-PROXY_DOMAIN>:6443`.
Объедините настройки с существующей секцией `https`, не создавая второй ключ.

Проверьте конфигурацию и запустите сервис из корня проекта:

```sh
docker compose config --quiet
docker compose up -d 2fauth https
```

При использовании Swarm каталог данных и его права подготовьте на узле
с меткой `node.labels.location == local`; переменные `TWOFAUTH_APP_URL` и
`TWOFAUTH_APP_KEY` передайте используемой командe рендеринга/развёртывания.

Документация: [установка Docker](https://docs.2fauth.app/getting-started/installation/docker/docker-cli/),
[переменные окружения](https://docs.2fauth.app/getting-started/config/env-vars/).
