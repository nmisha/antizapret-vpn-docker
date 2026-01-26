ИСПОЛЬЗУЕТСЯ ЭТОТ ВАРИАНТ




Ок. Делаем event-driven: systemd-сервис слушает docker events и при старте нужного контейнера запускает post-start фикс. Никаких таймеров/cron.

Нужно 2 юнита systemd и 1 скрипт.

0) Что именно фиксируем

Внутри контейнера wg-easy:

удалить неправильный NAT -o eth0

добавить универсальный MASQUERADE без -o

1) Скрипт фикса (oneshot)

Создай /usr/local/bin/wg-easy-poststart.sh на vps-nl-1:

cat >/usr/local/bin/wg-easy-poststart.sh <<'EOF'
#!/bin/sh
set -eu

SVC="antizapret_wg-easy-full"

# На воркере доступны лейблы swarm-сервиса, это самый надёжный поиск текущего контейнера
CID="$(docker ps -q --filter "label=com.docker.swarm.service.name=${SVC}" | head -n1 || true)"

# Если контейнера нет — просто выходим успехом
[ -n "$CID" ] || exit 0

# Применяем идемпотентно
docker exec "$CID" sh -c '
iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || true
iptables -t nat -C POSTROUTING -s 10.8.0.0/24 -j MASQUERADE 2>/dev/null || \
iptables -t nat -A POSTROUTING -s 10.8.0.0/24 -j MASQUERADE
' >/dev/null 2>&1 || true

exit 0
EOF

chmod +x /usr/local/bin/wg-easy-poststart.sh


(Скрипт безопасный: если контейнера нет — ничего не делает.)

2) systemd service-слушатель (долгоживущий)

Создай /etc/systemd/system/wg-easy-events.service:

cat >/etc/systemd/system/wg-easy-events.service <<'EOF'
[Unit]
Description=Listen Docker events and fix wg-easy NAT on start
After=docker.service
Requires=docker.service

[Service]
Type=simple

# Слушаем события старта контейнеров нужного swarm-сервиса.
# У контейнера будет лейбл com.docker.swarm.service.name=antizapret_wg-easy-full
ExecStart=/bin/sh -lc '\
docker events --format "{{json .}}" \
  --filter "event=start" \
  --filter "label=com.docker.swarm.service.name=antizapret_wg-easy-full" \
  | while read -r _; do /usr/local/bin/wg-easy-poststart.sh; done \
'

Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

3) Опционально: one-shot сервис для ручного запуска (удобно)

Можно, но не обязательно. Я советую — чтобы “пнуть” фикс вручную.

Создай /etc/systemd/system/wg-easy-poststart.service:

cat >/etc/systemd/system/wg-easy-poststart.service <<'EOF'
[Unit]
Description=Run wg-easy post-start NAT fix once
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/wg-easy-poststart.sh
EOF

4) Включить и запустить
systemctl daemon-reload
systemctl enable --now wg-easy-events.service


И сразу можно выполнить фикс вручную (для проверки):

systemctl start wg-easy-poststart.service

5) Проверка, что слушатель работает

Статус:

systemctl status wg-easy-events.service --no-pager


Логи:

journalctl -u wg-easy-events.service -n 100 --no-pager


Проверка NAT внутри текущего контейнера:

CID=$(docker ps -q --filter "label=com.docker.swarm.service.name=antizapret_wg-easy-full" | head -n1)
docker exec -it "$CID" iptables -t nat -S POSTROUTING | grep 10.8.0.0/24

Что будет происходить дальше автоматически

Swarm пересоздал контейнер wg-easy → Docker сгенерировал event=start

wg-easy-events.service это увидел → запустил /usr/local/bin/wg-easy-poststart.sh

скрипт поправил iptables внутри нового контейнера

Если хочешь, могу добавить в скрипт логирование через logger, чтобы в journalctl было видно “patched CID=...”, и легче отлаживать.