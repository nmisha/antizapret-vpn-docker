# Telemt Panel в Swarm

Пример адреса: https://panel.example.com (замените на свой домен). Caddy на `local` выпускает сертификат,
Authelia допускает группу `telemt_admins`. Второго входа нет: в конфиге панели
задано `[auth] disabled = true`. Все допущенные пользователи получают полные
права управления; персональные роли внутри панели не создаются.

Закреплён официальный prerelease-образ `ghcr.io/amirotin/telemt_panel:1.0.0-rc.2`.
Панель на `world` обращается к `http://telemt:9091`, порты хоста не публикуются.
Пользователи и настройки редактируются через API. Docker socket не подключён;
обновление образов и перезапуск служб выполняются средствами Swarm.

## Подготовка

### Автоматическая генерация при старте

Swarm передаёт `entrypoint.sh` как Docker config. Если
`config-mine/telemt-panel/config/config.toml` отсутствует, скрипт при старте
контейнера создаёт его из `TELEMT_PANEL_DOMAIN` и текущего API-токена telemt.
Он включает `[auth] disabled = true`, доверие сети `telemt-panel-auth` и SQLite,
проверяет TOML бинарником панели и только затем сохраняет файл. Существующий
конфиг не перезаписывается; состояние и история в `data` сохраняются.

Для первого запуска вместо ручного `prepare.py` достаточно создать каталоги
на world (из корня проекта), применить сеть и правило Authelia и развернуть стек:

```sh
sudo install -d -m 0700 config-mine/telemt-panel/config config-mine/telemt-panel/data
```

Каталог `config-mine/telemt` также должен существовать на world. Панель читает его
только для чтения, ожидая API-токен до 60 секунд; после ошибки Swarm может повторить
запуск. Поддерживается формат токена, создаваемый entrypoint telemt: `A-Za-z0-9._-`.
Скрипт подготовки на Python ниже остаётся альтернативой для ручного создания.

После удаления **только config.toml** конфиг будет восстановлен при следующем
старте задачи, а не немедленно в работающем процессе:

```sh
docker service update --force antizapret_telemt-panel
```

Пересборка образа не требуется. Скрипт Docker config доставляется с manager;
при изменении уже развёрнутого скрипта используйте новую версию имени Docker config
в обоих Compose-файлах (Swarm configs неизменяемы).

### Ручная подготовка (альтернатива)

1. Добавить DNS A/CNAME для `panel.example.com`, ведущую на Caddy (`local`).
2. Сначала запустить telemt без секции `telemt-panel` в серверном override.
   Дождаться создания `config-mine/telemt/config.toml` на `world`.
3. Перенести каталог `services-mine/telemt-panel` на `world` и из корня проекта выполнить:

   ```sh
   sudo python3 services-mine/telemt-panel/prepare.py --domain panel.example.com
   ```

   Нужны Python 3.11+ и Docker. Скрипт создаёт каталоги bind mount, читает текущий
   API-токен telemt и проверяет TOML бинарником панели. Отдельный пароль не создаётся.
   Каталог исключён из Git. Повторный запуск сохраняет существующий конфиг и завершается.
   Если API-токен telemt изменится, обновить `[telemt].auth_header` в конфиге панели.
4. Перенести Compose сервиса на manager. Добавить секцию `telemt-panel`,
   `SNI_ROUTE_7` и `PROXY_VHOST_3` из `config-docker-swarm/docker-compose.override.yml`
   в используемый на сервере override. Обычно это корневой `docker-compose.override.yml`,
   выбранный в `compose.swarm.env`.
5. Перенести правило `panel.example.com` из конфигурации Authelia на `local`.
   После проверки конфигурации применить стек и перезапустить Authelia для чтения правила.

Пути bind mount должны совпадать с абсолютным путём проекта на manager.
Данные панели лежат в `config-mine/telemt-panel/data`: сохранять каталог вместе
с конфигом для резервирования. При нескольких `world`-нодах закрепить telemt
и панель за узлом с данными; локальные bind mount не реплицируются.
`trusted_proxies` должен соответствовать вашей overlay-подсети;
при смене подсети нужно обновить конфиг панели.

## Переход существующей панели на Authelia

Сначала применить отдельную overlay-сеть `telemt-panel-auth` из Swarm override:
`https` и `telemt` подключены к ней дополнительно, а `telemt-panel` подключена
**только** к ней. Порты панели не публикуются. Эта сеть не attachable; подключать
к ней другие приложения нельзя: доступ к порту панели даёт права администратора.
Доверенными участниками остаются Caddy, telemt и администраторы Docker.

Затем применить правило Authelia для группы `telemt_admins` и добавить нужных
пользователей в эту группу. Членство в общей группе `admins` само по себе
больше не предоставляет доступ. Проверить конфигурацию и перезапустить Authelia.

После изоляции сети сделать резервную копию существующего TOML панели на world
и изменить **существующие** параметры (не добавлять вторую секцию `[auth]`):

```toml
# Верхний уровень, до секций:
public_url = "https://panel.example.com"
trusted_proxies = ["10.44.42.0/24"]

[auth]
disabled = true
```

Имеющиеся `username` и `password_hash` можно оставить для отката; они не
используются при `disabled = true`. `prepare.py` намеренно не перезаписывает
существующие конфиги. Проверить TOML через `telemt-panel config check`, затем
перезапустить только сервис панели. Отключение входа сохраняет CSRF-проверки;
`public_url` обязателен для правильной проверки Host. Состояние и SQLite не удалять.

## Проверка

```sh
docker service ps --no-trunc antizapret_telemt-panel
docker service logs --tail 100 antizapret_telemt-panel
curl -I https://panel.example.com/
```

Без сессии ожидается редирект на Authelia. После входа пользователя группы `telemt_admins` панель открывается без второго пароля.
Затем проверить список пользователей telemt. `502` означает недоступную панель,
`403` от Authelia — отсутствие правила или членства в `telemt_admins`.

Документация: https://github.com/amirotin/telemt_panel/blob/main/docs/DOCKER.md
