# Firewall Antizapret: блоклисты и исключения

Firewall работает в сетевом пространстве хоста (`network_mode: host`, в swarm — сеть `host`, `mode: global`)
и не пускает источники из блоклистов `V4_URL` / `V6_URL` к опубликованным портам контейнеров.
Реализация на bash совпадает с PR в upstream:
[#249](https://github.com/xtrime-ru/antizapret-vpn-docker/pull/249) (атомарная замена ipset) и
[#250](https://github.com/xtrime-ru/antizapret-vpn-docker/pull/250) (исключения).

## Как работает

- Правила — в собственной цепочке `AZ-FIREWALL`: `ESTABLISHED,RELATED → RETURN`, исключения `→ RETURN`,
  источники из `az_firewall_v4` / `az_firewall_v6` `→ DROP`. Цепочка загружается одной транзакцией
  `iptables-restore`; помеченный переход (`--comment antizapret-firewall`) стоит первым правилом `DOCKER-USER`.
- `RETURN`, а не `ACCEPT`: последующие правила Docker, UFW и другие продолжают действовать.
- Обновление списков: `ipset restore` во временный набор `<set>_next`, затем `ipset swap`. Во время обновления
  защита не пропадает; если список не загрузился, действующий набор не меняется.
- Строки с комментариями, IPv6 в v4-списке, октеты > 255, неверные префиксы пропускаются (в логе — количество).
  Список без единой валидной строки не применяется, запуск завершается ошибкой и повторяется через 30 с.
- Семейства обновляются независимо: ошибка в IPv6-списке не мешает обновить IPv4.
- Загрузка идёт во временный файл, недокачанный список не заменяет предыдущий.
- Запуски `block.sh` сериализованы через `flock`: ручной `block.sh` не пересекается с плановым обновлением.
- При штатной остановке trap вызывает `block.sh clear`: удаляются переход, цепочка и ipset.

## Исключения

Нужны, когда в блоклист попали, например, SMTP-серверы Mail.ru, а почтовый сервер должен принимать от них почту.
Формат строки (`exceptions.example`):

```text
интерфейс IP-назначения tcp|udp порт[,порт...]
ens3 203.0.113.10 tcp 25,80,443,465,587,993,4190
ens3 2001:db8::10 udp 443
```

- IP-назначения — публичный адрес хоста (один IPv4/IPv6, без CIDR), порты — опубликованные, как до Docker DNAT:
  сравнение идёт по исходному адресу назначения из conntrack.
- Файл проверяется до любых изменений; неверная строка выводится с номером, действующие правила сохраняются.
- Реальный файл с адресами серверов в репозиторий и образ **не кладётся**: он лежит в каталоге конфигурации
  на хостах (`config/` в `.gitignore`) и подключается в контейнер только для чтения.

Подключение в swarm (firewall работает на всех узлах, `mode: global`):

1. Положить **одинаковый** файл на **каждый** узел: `/root/antizapret6-swarm/config/firewall/exceptions`.
   Если файла не будет на каком-то узле, обновление там завершится ошибкой «Invalid EXCEPTIONS_FILE»
   (действующие правила сохранятся, но блоклисты перестанут обновляться). На узлах, где адресов из файла нет,
   правила ничего не пропускают.
2. В `docker-compose.override.yml`:
   ```yaml
   firewall:
     volumes:
       - $PWD/config/firewall:/root/exceptions.d:ro
     environment:
       - EXCEPTIONS_FILE=/root/exceptions.d/exceptions
   ```

Применить изменённый файл сразу: `docker exec <firewall-container> /root/block.sh` (на каждом узле).

## Переход с Python-версии (`firewall.py`, цепочка `AZ-VPN-FILTER`)

Имена ipset у старой и новой версии одинаковые — запускать их одновременно нельзя. В swarm нужен
`deploy.update_config.order: stop-first`.

1. Штатная остановка старого контейнера сама вызывает `clear` и удаляет `AZ-VPN-FILTER`.
2. Если старый контейнер был убит (SIGKILL) и цепочка осталась:
   ```sh
   for t in iptables ip6tables; do
     while $t -D DOCKER-USER -m comment --comment antizapret-firewall -j AZ-VPN-FILTER 2>/dev/null; do :; done
     $t -F AZ-VPN-FILTER 2>/dev/null; $t -X AZ-VPN-FILTER 2>/dev/null
   done
   ```
3. Проверить после запуска новой версии:
   ```sh
   iptables -S DOCKER-USER | head -3     # первое правило — переход в AZ-FIREWALL
   iptables -S AZ-FIREWALL
   ipset list -t az_firewall_v4          # Number of entries
   ```

## Сборка

Нужен только образ firewall:

```sh
docker buildx build --platform linux/amd64,linux/arm64 \
  -t nmisha/antizapret-vpn-firewall:6.4.0-2 --push services/firewall
```
