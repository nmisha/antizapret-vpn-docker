Вариант 1 — “универсальный” MASQUERADE (не зависит от eth0/eth1)

Это лучший вариант для Swarm, где интерфейсы могут отличаться:

В /etc/wireguard/wg0.conf:

[Interface]
# ...
PostUp   = iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -j MASQUERADE
PostDown = iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -j MASQUERADE


Плюсы:

не важно, eth0 или eth1

работает одинаково в compose и swarm

Минус:

если внутри контейнера несколько аплинков, NAT будет применяться “везде” (обычно это ок).

Вариант 2 — “строго на eth1” (под твой текущий Swarm)
PostUp   = iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -o eth1 -j MASQUERADE
PostDown = iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth1 -j MASQUERADE


Плюс: максимально точечно.
Минус: если завтра egress станет через другой интерфейс — снова сломается.

Важный момент: сначала убрать “неправильное” правило wg-easy

Сейчас wg-easy уже создаёт:
-A POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE

Если ты добавишь новое, будет два правила (одно мёртвое, одно рабочее) — в целом не страшно, но лучше привести к одному.

Самый простой путь — в PostUp сначала удалить (если есть), потом добавить правильное:

PostUp   = iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || true; iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -j MASQUERADE
PostDown = iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -j MASQUERADE 2>/dev/null || true


(это “грязновато”, но практично)


=========================

PostUp   = iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -j MASQUERADE
PostDown = iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -j MASQUERADE


wg-easy сам же генерирует /etc/wireguard/wg0.conf. и правила перепишет. как их зафиксировать?

Отличный и очень правильный вопрос.
Короткий ответ: не лезть напрямую в wg0.conf, потому что wg-easy его перегенерирует, и твои правки действительно пропадут.

Ниже — рабочие способы “зафиксировать” правила, которые переживают регенерацию wg-easy. От лучшего к худшему.

🥇 Лучший и надёжный способ
Вынести NAT/FORWARD на хост (а не в контейнер)

Это самый устойчивый вариант для Swarm.

Почему:

wg-easy может переписывать wg0.conf

контейнеры могут пересоздаваться

а iptables на хосте — под твоим полным контролем

Минимальный набор на хосте
iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -o ens3 -j MASQUERADE
iptables -I FORWARD 1 -s 10.8.0.0/24 -j ACCEPT
iptables -I FORWARD 1 -d 10.8.0.0/24 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT


И закрепить персистентно (systemd / nftables / iptables-persistent).

✔ не зависит от wg-easy
✔ не зависит от eth0/eth1
✔ одинаково работает в compose и swarm

🥈 Второй вариант (рабочий): подменить хуки через wg-quick wrapper

wg-easy под капотом использует wg-quick up wg0.

wg-quick поддерживает внешние хуки через:

/etc/wireguard/*.conf (но они переписываются)

или через system hooks, если запускать вручную (wg-easy — не тот случай)

➡️ В контейнере нет стабильной точки, куда можно “вклиниться”, не рискуя регенерацией.

Поэтому в контейнере это ненадёжно.

🥉 Костыль (иногда используют): скрипт, который исправляет правила после старта

Идея:

дать контейнеру стартовать

затем удалить “неправильное” правило и добавить правильное

Пример entrypoint-wrapper (НЕ рекомендую в Swarm, но возможно)
#!/bin/sh
sleep 5
iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || true
iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -j MASQUERADE


Минусы:

race conditions

обновления wg-easy могут сломать

сложно отлаживать

🧠 Почему ENV не спасают

В wg-easy нет ENV, позволяющего:

выбрать egress-интерфейс для NAT

или сказать “не добавляй свой MASQUERADE”

Это известная особенность, а не твоя ошибка.

🎯 Рекомендую именно тебе (по твоему сетапу)

У тебя:

Swarm

overlay + docker_gwbridge

нестабильный egress интерфейс (eth1)

CrowdSec / DOCKER-USER

👉 NAT и FORWARD должны жить на хосте.

Контейнер пусть только:

поднимает wg0

принимает UDP 5182x

маршрутизирует пакеты внутрь

Готовый “чистовой” вариант для хоста (systemd unit)
# /etc/systemd/system/wg-nat.service
[Unit]
After=docker.service
Wants=docker.service

[Service]
Type=oneshot
ExecStart=/sbin/iptables -t nat -C POSTROUTING -s 10.8.0.0/24 -o ens3 -j MASQUERADE || \
          /sbin/iptables -t nat -A POSTROUTING -s 10.8.0.0/24 -o ens3 -j MASQUERADE
ExecStart=/sbin/iptables -C FORWARD -s 10.8.0.0/24 -j ACCEPT || \
          /sbin/iptables -A FORWARD -s 10.8.0.0/24 -j ACCEPT
ExecStart=/sbin/iptables -C FORWARD -d 10.8.0.0/24 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT || \
          /sbin/iptables -A FORWARD -d 10.8.0.0/24 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target

systemctl daemon-reexec
systemctl enable --now wg-nat

Кратко

❌ Править wg0.conf — бесполезно (wg-easy перегенерирует)

❌ Надеяться на ENV — некуда

✅ Делать NAT/FORWARD на хосте — правильно и стабильно

🧘 wg-easy пусть занимается только WG

Если хочешь — могу:

переписать это сразу под nftables

или аккуратно вписать WG в твой CrowdSec / DOCKER-USER

или собрать финальный docker stack deploy под host-network без лишних дыр

Скажи, в какую сторону идём дальше.


=============================================================================================


1) Команда “всегда найти текущий wg-easy контейнер”
CID=$(docker ps -q --filter "label=com.docker.swarm.service.name=antizapret_wg-easy-full" | head -n1)
echo "CID=$CID"

2) Применить фикс NAT внутри контейнера (убрать -o eth0, сделать универсально)

Скопируй и выполни:

CID=$(docker ps -q --filter "label=com.docker.swarm.service.name=antizapret_wg-easy-full" | head -n1)

docker exec -it "$CID" sh -c '
echo "egress:"; ip route get 1.1.1.1 || true; echo;

echo "before:"; iptables -t nat -L POSTROUTING -v -n --line-numbers; echo;

# убрать неправильное правило wg-easy (если есть)
iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || true

# добавить правильное (без привязки к интерфейсу)
iptables -t nat -C POSTROUTING -s 10.8.0.0/24 -j MASQUERADE 2>/dev/null || \
iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -j MASQUERADE

echo "after:"; iptables -t nat -L POSTROUTING -v -n --line-numbers
'

Проверка

С клиента:

ping 1.1.1.1


Счётчики должны расти:

CID=$(docker ps -q --filter "label=com.docker.swarm.service.name=antizapret_wg-easy-full" | head -n1)
docker exec -it "$CID" iptables -t nat -L POSTROUTING -v -n --line-numbers

3) Сделать так, чтобы фикс переживал пересоздание контейнера (systemd timer на воркере)
Скрипт
cat >/usr/local/sbin/wg-easy-nat-fix.sh <<'EOF'
#!/bin/sh
set -eu

SVC="antizapret_wg-easy-full"
CID="$(docker ps -q --filter "label=com.docker.swarm.service.name=$SVC" | head -n1 || true)"
[ -n "$CID" ] || exit 0

docker exec "$CID" sh -c '
iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || true
iptables -t nat -C POSTROUTING -s 10.8.0.0/24 -j MASQUERADE 2>/dev/null || \
iptables -t nat -I POSTROUTING 1 -s 10.8.0.0/24 -j MASQUERADE
' >/dev/null 2>&1 || true
EOF

chmod +x /usr/local/sbin/wg-easy-nat-fix.sh

Unit + timer
cat >/etc/systemd/system/wg-easy-nat-fix.service <<'EOF'
[Unit]
Description=Fix wg-easy MASQUERADE rule inside current Swarm task
After=docker.service
Wants=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/wg-easy-nat-fix.sh
EOF

cat >/etc/systemd/system/wg-easy-nat-fix.timer <<'EOF'
[Unit]
Description=Run wg-easy NAT fix periodically

[Timer]
OnBootSec=30s
OnUnitActiveSec=30s

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now wg-easy-nat-fix.timer


Проверка:

systemctl list-timers | grep wg-easy-nat-fix

Важное замечание по безопасности (по твоему inspect)

В выводе лейблов у тебя светится пароль виджета:
homepage.widget.password:""

Это не критично для NAT, но это прямо хранится в метаданных контейнера (и видно всем, кто может сделать docker inspect). Если это реальный пароль — лучше сменить/убрать из labels.

