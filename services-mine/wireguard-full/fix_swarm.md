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