cat >/etc/sysctl.d/99-wireguard.conf <<'EOF'
net.ipv4.conf.all.src_valid_mark=1
net.ipv4.ip_forward=1
EOF
sysctl --system


- net.ipv6.conf.all.disable_ipv6=0
- net.ipv6.conf.all.forwarding=1
- net.ipv6.conf.default.forwarding=1


cat >/etc/sysctl.d/99-wireguard.conf <<'EOF'
net.ipv4.conf.all.src_valid_mark=1
net.ipv4.ip_forward=1
net.ipv6.conf.all.disable_ipv6=0
net.ipv6.conf.all.forwarding=1
net.ipv6.conf.default.forwarding=1
EOF
sysctl --system



docker service ps antizapret_wg-easy-full --no-trunc
docker service update --force antizapret_wg-easy-full

=====

Это отлично работает:
-e WG_POST_UP="iptables -A FORWARD -i %i -j ACCEPT;iptables -A FORWARD -o %i -j ACCEPT;iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE" \ 
-e WG_POST_DOWN="iptables -D FORWARD -i %i -j ACCEPT;iptables -D FORWARD -o %i -j ACCEPT;iptables -t nat -D POSTROUTING -o eth0" \

Тем не мене

environment:
  - WG_POST_UP=iptables -A FORWARD -i %i -j ACCEPT; iptables -A FORWARD -o %i -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
  - WG_POST_DOWN=iptables -D FORWARD -i %i -j ACCEPT; iptables -D FORWARD -o %i -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE


WG_POST_UP в файле docker-compose.yml, указав нужные iptables правила (например, iptables -A FORWARD -i %i -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE), и пересоздать контейнер.






