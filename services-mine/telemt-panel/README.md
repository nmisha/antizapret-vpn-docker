# Telemt Panel в Swarm

Пример адреса: https://panel.example.com (замените на свой домен). Caddy на `local` выпускает сертификат,
Authelia допускает группу `admins`. После этого панель
запрашивает собственный логин `admin` и отдельный пароль: SSO через заголовки
Authelia в этой конфигурации не реализован.

Закреплён официальный prerelease-образ `ghcr.io/amirotin/telemt_panel:1.0.0-rc.2`.
Панель на `world` обращается к `http://telemt:9091`, порты хоста не публикуются.
Пользователи и настройки редактируются через API. Docker socket не подключён;
обновление образов и перезапуск служб выполняются средствами Swarm.

## Подготовка

1. Добавить DNS A/CNAME для `panel.example.com`, ведущую на Caddy (`local`).
2. Сначала запустить telemt без секции `telemt-panel` в серверном override.
   Дождаться создания `config-mine/telemt/config.toml` на `world`.
3. Перенести каталог `services-mine/telemt-panel` на `world` и из корня проекта выполнить:

   ```sh
   sudo python3 services-mine/telemt-panel/prepare.py --domain panel.example.com
   ```

   Нужны Python 3.11+ и Docker. Скрипт создаёт каталоги bind mount, читает текущий
   API-токен telemt, генерирует отдельный пароль и проверяет TOML бинарником панели.
   Пароль хранится в `config-mine/telemt-panel/initial-login.txt` с правами 0600.
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

## Проверка

```sh
docker service ps --no-trunc antizapret_telemt-panel
docker service logs --tail 100 antizapret_telemt-panel
curl -I https://panel.example.com/
```

Без сессии ожидается редирект на Authelia. После входа пользователя группы `admins` — вход панели.
Затем проверить список пользователей telemt. `502` означает недоступную панель,
`403` от Authelia — отсутствие правила или членства в `admins`.

Документация: https://github.com/amirotin/telemt_panel/blob/main/docs/DOCKER.md
