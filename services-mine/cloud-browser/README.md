# Chromium в Swarm

Адрес: **https://cb.example.com** (замените на свой домен). Caddy и Authelia работают на `local`,
Chromium — одна реплика на `world`, связь через общую overlay-сеть.
Доступ разрешён группе Authelia `cloud_browser` с `one_factor`; в группу добавлен `mi`.
Все допущенные пользователи работают с одним общим профилем.

## Перенос

1. Перенести `services-mine/cloud-browser/docker-compose.yml` в репозиторий
   на сервере. Взять подготовленный `config-docker-swarm/docker-compose.override.yml`
   и положить его в **корень серверного репозитория** как `docker-compose.override.yml`:
   именно этот путь использует `compose.swarm.env`. Сверить параллельные изменения.
2. Перенести `config-mine/authelia/config/configuration.yml` и
   `config-mine/authelia/config/users_database.yml` на узел `local`
   в существующий каталог конфигурации Authelia.
3. DNS `cb.example.com` направить на публичный адрес Caddy (`local`).
   Для выпуска сертификата нужны доступные TCP 80/443.
4. На узле `world` создать каталог профиля с владельцем `1000:1000`.
   По умолчанию путь — `<абсолютный путь репозитория на manager>/config-mine/chromium`.
   `$PWD` подставляется на manager при подготовке стека, поэтому этот же абсолютный
   путь должен существовать на `world`. При разных путях перед генерацией стека
   задать на manager `export CHROMIUM_CONFIG_DIR=/absolute/path/on/world/chromium`.

   На `world`, подставив выбранный абсолютный путь:

   ```sh
   sudo install -d -o 1000 -g 1000 -m 0750 /absolute/path/on/world/chromium
   ```

5. На manager проверить метки узлов: `docker node inspect <world-node> --format '{{json .Spec.Labels}}'`.
   Нужна `location=world`. Если таких узлов несколько, закрепить сервис за одним
   конкретным узлом до запуска: bind mount не переносит профиль между узлами.

## Проверка и применение

Из корня серверного репозитория на manager (Bash):

```sh
set -o pipefail
umask 077
docker compose --env-file compose.swarm.env config \
  | docker run --pull always --rm -i xtrime/antizapret-vpn:6 compose2swarm \
  > /tmp/cloud-browser.stack.yml
docker stack config -c /tmp/cloud-browser.stack.yml >/dev/null
```

Применять только после успешной проверки всего стека:

```sh
docker stack deploy --prune -c /tmp/cloud-browser.stack.yml antizapret
docker service update --force antizapret_authelia
rm /tmp/cloud-browser.stack.yml
docker service ps antizapret_chromium
docker service logs --tail 100 antizapret_chromium
```

Принудительное обновление Authelia нужно для перечитывания bind-mounted конфигурации.
Сгенерированный файл содержит настройки и секреты всего стека; не добавлять его в Git.
Новые образы Caddy для этих изменений собирать не требуется, если на сервере уже
используется версия с поддержкой `PROXY_VHOST_N` из текущей конфигурации.

## Приёмка на сервере

- Без входа открывается Authelia; пользователь вне `cloud_browser` не получает доступ.
- После входа работают изображение, клавиатура, мышь и переходы по сайтам.
- WebSocket удерживает соединение; после закрытия вкладки можно подключиться снова.
- После `docker service update --force antizapret_chromium` сохраняется профиль
  (проверить закладкой); восстановление вкладок зависит от настроек Chromium.
- На `world` внутри контейнера `df -h /dev/shm` показывает 2 ГиБ.
- `docker service inspect antizapret_chromium --format '{{json .Endpoint.Spec.Ports}}'`
  не показывает опубликованных портов.
- Внешний сайт определения IP показывает ожидаемый выход узла `world`.

## Выполненные локальные проверки

Compose с указанным Swarm override успешно объединяется; Firefox не включён.
Преобразование Chromium повторно проверено образом из скрипта запуска `xtrime/antizapret-vpn:6`;
отдельная конфигурация Chromium проходит `docker stack config`.
Полный стек при проверке остановился на существующем `services.telemt.ports.0.host_ip`;
параллельно редактируемый telemt не изменялся. Полный актуальный стек
необходимо проверить приведённой выше командой перед развёртыванием.

Генератор Caddy и `caddy validate` проверены с текущими переменными и тестовыми
самоподписанными сертификатами без сети. Authelia 4.39.20 успешно проверила конфигурацию.
Реальная выдача сертификата, межузловая сеть и работа GUI проверяются после переноса.

Вместо `shm_size` используется `tmpfs: /dev/shm:size=2147483648,mode=1777`:
эта запись проходит текущий конвертер и парсер Swarm. Длинная форма `tmpfs.size`
после преобразования получала строковый тип и не проходила `docker stack config`.

`restart: unless-stopped` сохраняет принятый в проекте способ задания политики.
Проверенный `xtrime/antizapret-vpn:6 compose2swarm` преобразует его в
`condition: any`, `delay: 1s`, `max_attempts: 10`, `window: 5s`.
Собственный `build` Chromium не нужен: в каталоге нет Dockerfile, используется
готовый образ LinuxServer. В других сервисах healthcheck задаётся явно в Dockerfile
или в Compose; `build.x-bake.platforms` сам по себе healthcheck не добавляет.
Отдельным примером проверено, что конвертер сохраняет Compose healthcheck без `build`.

Источники: [LinuxServer Chromium](https://docs.linuxserver.io/images/docker-chromium/),
[Docker Compose tmpfs](https://docs.docker.com/reference/compose-file/services/#tmpfs).
