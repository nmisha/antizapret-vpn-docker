# Выход к IPv6-сайтам через TAYGA

Опциональный режим для IPv4-клиентов VPN и двух выходов Swarm `az-local` /
`az-world`. Основная конфигурация продолжает работать без NAT46. Образы нужно
собрать из этой ветки: опубликованный основной образ не содержит этих изменений.
На сервере TAYGA устанавливать не нужно: пакет находится в образе antizapret.
Внешние модули ядра, Jool и DKMS не нужны. Нужен штатный Linux TUN.

## Схема и выбор выхода

```text
IPv4-клиент → synthetic IPv4 → TAYGA /96 → DNAT6 к настоящему IPv6 сайта
  → MASQUERADE внутри контейнера → macvlan → host NAT66 → IPv6 ноды
```

Существующие правила AdGuard / ASN сначала выбирают local или world.
Затем выбранный выход проверяет IPv6-список: для включённого домена запрашивает
AAAA с прежним AdGuard client ID, устанавливает DNAT6 и возвращает synthetic A.
Обычные IPv4-назначения продолжают использовать прежний IPv4 DNAT.

Контейнер имеет отдельный network namespace. IPv6-адрес и маршрут ноды в него
автоматически не попадают. Дополнительная **локальная macvlan-сеть** даёт путь
от контейнера к хосту; она не переносит IPv6 между нодами. При NAT66 публичный
адрес выхода — адрес соответствующей ноды. Существующий IPv4 overlay сохраняется.
IPv4 default route явно сохраняется через `docker_gwbridge`, потому что при
добавлении macvlan Docker может выбрать другой gateway.

## 1. Проверки на каждой ноде выхода

Выполнить самостоятельно на local и world:

```sh
ip -6 address show scope global
ip -6 route show default
curl -6 --connect-timeout 10 https://example.com/
docker network inspect docker_gwbridge --format '{{json .IPAM.Config}}'
ip -4 route
sudo modprobe tun
```

Записать IPv4 gateway `docker_gwbridge` отдельно для каждой ноды и интерфейс
IPv6 default route. Проверить отсутствие пересечений с техническими подсетями:
`fd46:64::/96` (трансляция), `fd46:65::/64` (TUN), `fd46:ee::/64` и
`172.31.46.0/24` (канал контейнер—хост), `192.0.2.0/24` (внутренние адреса TAYGA).
IPv6 egress /64 и его gateway можно изменить в host-конфиге. Translation /96
можно изменить в Compose; он должен совпадать у контроллера и dnsmap.

Нужны Docker Swarm с поддержкой `cap_add` и service sysctls, `iproute2`,
`iptables` с IPv6 NAT и `python3` на хосте. На Ubuntu/Debian:

```sh
sudo apt-get update
sudo apt-get install -y iproute2 iptables python3
```

Включить постоянный `net.ipv6.conf.all.forwarding=1`. Если default route приходит
через Router Advertisement, **до включения forwarding** настроить
`net.ipv6.conf.<WAN>.accept_ra=2` для своего интерфейса и проверить обновление
маршрута. Эти параметры зависят от сети провайдера; host-скрипт проверяет
forwarding и маршрут, но не меняет их. При необходимости добавить `tun` в
`/etc/modules-load.d/antizapret-tun.conf`.

## 2. IPv6-канал и NAT66 на хостах

Из корня проекта на **каждой** ноде выхода:

```sh
sudo install -m 755 scripts/nat46-egress-host.sh /usr/local/sbin/antizapret-nat46-egress
sudo install -m 644 scripts/antizapret-nat46.example /etc/default/antizapret-nat46
sudoedit /etc/default/antizapret-nat46
sudo install -m 644 scripts/antizapret-nat46.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now antizapret-nat46.service
```

В конфиге заменить `NAT46_WAN_INTERFACE=REPLACE_ME` своим интерфейсом.
Изначально оставить `NAT46_HOST_NAT66=1`. Скрипт создаёт `az46-parent` (dummy),
`az46-shim` (macvlan с адресом gateway), локальную config-only Docker-сеть,
цепочки `AZ_NAT46` и `AZ_NAT46_FWD`. Существующие цепочки Docker не очищаются.
Unit повторяет настройку при старте Docker. После ручного перезагрузки firewall
повторить `systemctl reload antizapret-nat46`.

После настройки всех нод **один раз на manager**:

```sh
docker network create --driver macvlan --scope swarm \
  --config-from antizapret-nat46-egress-config antizapret-nat46-egress
```

На каждой подходящей по placement ноде должен быть config-only с этим именем.
При изменении IPAM сначала остановить использующие сеть сервисы, затем
пересоздать внешнюю сеть и config-only; скрипт намеренно отказывается молча
использовать несовместимую конфигурацию. Не запускать одновременно старый
Jool-вариант с теми же именами host-ресурсов: переход требует их удаления после
остановки старых задач. Миграция `.nat46.rules` старого варианта не поддерживается: новый формат
`nat46-state-v1` содержит оба семейства. Сохранить резервную копию состояния,
остановить старые выходы и дождаться истечения DNS-кешей перед очисткой старых
файлов и переходом.

`NAT46_HOST_NAT66=0` отключает **только NAT на хосте**. Для этого нужен отдельный
публичный /64, маршрутизируемый провайдером к этой ноде: задать его как
`NAT46_EGRESS_SUBNET`, адрес gateway из него и
`NAT46_ROUTED_PREFIX_CONFIRMED=1`. На разных нодах нужны отдельные routed /64.
Просто наличие публичного IPv6 у сервера этого не гарантирует. Внутренний
MASQUERADE контейнера остаётся частью схемы; индивидуальные адреса VPN-клиентов
снаружи не сохраняются.

## 3. Сборка и Swarm

Пересобрать **antizapret и AdGuard**. CoreDNS, VPN-контейнеры и firewall-образ
для этого изменения пересобирать не требуется. Пример с собственным registry:

```sh
export NAT46_IMAGE=registry.example.com/antizapret:tayga
export NAT46_ADGUARD_IMAGE=registry.example.com/adguard:tayga
docker build -t "$NAT46_IMAGE" services/antizapret
docker build --build-arg ROUTES_IMAGE="$NAT46_IMAGE" \
  -t "$NAT46_ADGUARD_IMAGE" services/adguard
docker push "$NAT46_IMAGE"
docker push "$NAT46_ADGUARD_IMAGE"
```

На manager экспортировать фактические значения:

```sh
export NAT46_LOCAL_IPV4_GATEWAY=LOCAL_DOCKER_GWBRIDGE_GATEWAY
export NAT46_WORLD_IPV4_GATEWAY=WORLD_DOCKER_GWBRIDGE_GATEWAY
export NAT46_OVERLAY_SUBNET=10.200.0.0/24
```

Значение overlay сверить со своей существующей сетью. Это подсеть исходного
IPv4 overlay проекта, а не дополнительной macvlan. В привычную команду
`docker compose ... config` добавить `-f compose.nat46.yml` **последним**,
после Swarm и персональных overrides. Далее использовать штатный
`compose2swarm` и `docker stack deploy` проекта. Перед deploy проверить
полученную конфигурацию через `docker stack config`: образы — новые,
`net.ipv6.conf.all.forwarding=1`, обе сети у выходов, persistence включён.

Пример подготовки файла для проверки (если персонального override нет, убрать
соответствующий `-f`):

```sh
docker compose --env-file compose.swarm.env \
  -f docker-compose.yml -f compose.swarm.yml -f docker-compose.override.yml \
  -f compose.nat46.yml config --format json > /tmp/antizapret-compose.json
docker run --rm -i "$NAT46_IMAGE" compose2swarm \
  < /tmp/antizapret-compose.json > /tmp/antizapret-stack.yml
docker stack config -c /tmp/antizapret-stack.yml
# Когда конфигурация проверена:
docker stack deploy --prune -c /tmp/antizapret-stack.yml antizapret
```

Override требует явные теги образов и оба IPv4 gateway, включает NAT46 на
выходах и AdGuard. Переменные host NAT66 читаются из host-конфига, а не Compose.
Обновления выходов — `stop-first`, по одной реплике; состояние хранится в
существующем bind mount `config/antizapret/iptables` каждой ноды. Не запускать
две реплики с одним пулом и state-файлами. Каталог должен сохраняться между
пересозданиями и быть доступен на соответствующей ноде.

## 4. Домены и DNS

В общем custom-каталоге создать/изменить:

```text
include-hosts-ipv6-custom.txt
exclude-hosts-ipv6-custom.txt
```

Например, добавить `example.com` в include. Правило включает домен и его
поддомены; поддерживаются Python regex в формате `/pattern/`. Exclude имеет
приоритет. Файлы перечитываются автоматически. Это выбор семейства адресов,
а выбор local/world остаётся в существующих include-hosts/ASN/AdGuard правилах.
Для первого теста выбрать домен, который уже направляется через нужный выход.

AdGuard разрешает внутренние AAAA-запросы для `az-local`, `az-world`,
`az-resolver`; для обычных клиентов и CoreDNS AAAA остаются выключены управляемым
правилом. Пользовательские дополнительные правила AAAA могут влиять на результат.
CNAME запрашиваются с тем же client ID, блокировки не обходятся. Если включённый
домен не даёт пригодного AAAA, ответ — SERVFAIL; автоматического fallback на IPv4
нет. Отмена include влияет на новые DNS-ответы, а прежние mappings остаются.

## Проверка и эксплуатация

На каждой ноде найти свой task-контейнер и проверить:

```sh
docker exec CONTAINER ip -6 route
docker exec CONTAINER ip -4 route show default
docker exec CONTAINER cat /proc/sys/net/ipv6/conf/all/forwarding
docker exec CONTAINER ip6tables -t nat -S nat46
docker exec CONTAINER cat /dev/shm/.nat46.pid
sudo ip6tables -t nat -S AZ_NAT46
```

С VPN-клиента запросить A выбранного домена и подключиться по HTTPS; адрес A
должен принадлежать synthetic-пулу соответствующего выхода. Проверить IPv6
счётчики и журналы на выбранной ноде. Затем проверить обычный IPv4-домен,
TCP, UDP/QUIC, перезапуск задачи и сохранение mappings. DNS-ответ выдаётся после
подтверждения правила ядром и атомарного общего checkpoint IPv4/IPv6 mappings. При старте этот снимок
авторитетен для обеих таблиц; отдельный старый IPv4 `.rules` нужен для обычного
режима и первичного импорта, когда общего снимка ещё нет.

Healthcheck проверяет процесс TAYGA и таблицу; PID1 завершает контейнер при
смерти DNS или TAYGA. При заполнении пула новые mappings получают ошибку,
существующие не сбрасываются. Mapping не удаляется по DNS TTL. Сброс состояния
требует остановки выхода и истечения DNS-кешей клиентов (synthetic TTL 300 с),
иначе старый synthetic IP может начать означать другой сайт.

## Ограничения и откат

- Linux TUN и два слоя conntrack/NAT66; скорость и CPU под реальной нагрузкой
  пока не измерены. TAYGA работает в userspace.
- Проверки локальные: контейнеры, одноузловой Swarm, контролируемый IPv6-сервер.
  Реальные VPN-туннели, два узла и IPv6 провайдера проверяются при твоём деплое.
- WARP вместе с NAT46 пока запрещён при старте. IPv6 для самих VPN-клиентов,
  входящие IPv6-соединения и сохранение публичных адресов клиентов не добавлены.
- Трансляция меняет DNS-ответ: DNSSEC AD снимается. Поддержаны обычные TCP/UDP
  и ICMP в пределах TAYGA; приложения с IP в payload могут быть несовместимы.
- Неизвестные synthetic-назначения отбрасываются, чтобы исключить TUN-петлю.
  PMTU зависит от прохождения ICMPv6 Packet Too Big; не блокировать его на WAN.
- При смене ноды state bind mount не переезжает автоматически. Закрепить local
  и world placement; для миграции отдельно переносить состояние остановленного
  выхода. Conntrack и уже установленные соединения при рестарте не сохраняются.

Откат: убрать override NAT46, вернуть обычные образы, пересоздать сервисы
(включая AdGuard, который возвращает глобальный запрет AAAA). Дождаться очистки
кешей synthetic A. После удаления задач из macvlan можно убрать внешнюю сеть,
остановить/отключить host unit, удалить его собственные hooks/цепочки и
`az46-shim` / `az46-parent`, затем config-only на нодах. Остановка unit сама
firewall и сети не удаляет. Не удалять другие Docker-правила или host forwarding,
если их используют другие сервисы. Сохранённые mappings оставить для диагностики.
