Создай файл, например:

/usr/local/bin/wg-easy-poststart.sh

----
#!/bin/sh
set -e

CID=$(docker ps -q --filter "name=antizapret_wg-easy-full.1" | head -n1)
[ -z "$CID" ] && exit 0

docker exec "$CID" sh -c '
iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || true
iptables -t nat -C POSTROUTING -s 10.8.0.0/24 -j MASQUERADE 2>/dev/null \
  || iptables -t nat -A POSTROUTING -s 10.8.0.0/24 -j MASQUERADE
'
-----

chmod +x /usr/local/bin/wg-easy-poststart.sh

-----

4️⃣ systemd-unit (НЕ костыльный, а нормальный)

Это не “каждые 30 секунд”, а аккуратный сервис, который:

ждёт Docker

запускается при старте

можно перезапустить вручную

/etc/systemd/system/wg-easy-poststart.service

-------
[Unit]
Description=Fix wg-easy NAT after container start
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/wg-easy-poststart.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
--------

Активировать:

systemctl daemon-reload
systemctl enable wg-easy-poststart.service
systemctl start wg-easy-poststart.service


