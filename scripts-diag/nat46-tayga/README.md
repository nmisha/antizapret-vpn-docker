# Эксперимент: статический TAYGA + динамический DNAT6

Ветка `experiment/nat46-tayga` создана от
`57c12e4ccb6c2b2e2b7b1a88e3796c4a35c0067e`.
Здесь проверяется сетевая схема; production DNS/Compose пока не изменены.
Jool-реализация сохранена отдельно в `feature/nat46-jool`.

## Схема

```text
IPv4-клиент -> synthetic IPv4
  -> TUN -> TAYGA: IPv4 <-> технический IPv6 (/96)
  -> ip6tables DNAT6: технический IPv6 -> настоящий IPv6 назначения
  -> container NAT66: адрес клиента -> IPv6 интерфейса контейнера
  -> host NAT66: IPv6 контейнера -> публичный IPv6 ноды
  -> IPv6-сервер
```

TAYGA имеет постоянную конфигурацию без динамических map-файлов. Меняются
только правила `ip6tables`: команда установки подтверждается ядром. Ни reload,
ни управление EAMT не нужны. Обратный трафик проходит обратные преобразования
conntrack и TAYGA. Обычный IPv4 DNAT выполняется до выбора маршрута в TUN.
Неизвестные synthetic IPv6 отклоняются, чтобы исключить петлю через TUN.

## Зачем дополнительная сеть, если выход через ноду

Публичный адрес выхода действительно принадлежит ноде local/world. Но у
контейнера собственный network namespace. Адрес и маршрут хоста ему
автоматически не передаются. Сейчас приложение использует IPv4 overlay и
IPv4 docker_gwbridge; это обеспечивает IPv4-выход, а не IPv6-выход.

В стенде отдельный macvlan на внутреннем dummy-интерфейсе и host shim дают
контейнеру ULA-адрес/default route до хоста. Далее host NAT66 использует адрес
ноды. Эта сеть не переносит IPv6 между нодами и не требует выдавать публичный
адрес контейнеру. Namespace выхода остаётся изолированным от host network.

Можно строить другой транспорт до хоста (например, туннель через IPv4), но
это отдельный механизм, не автоматическое следствие размещения на ноде.
В этом эксперименте существующий IPv4 overlay сохраняется.

## Что стало проще относительно Jool-варианта

- Нет внешнего модуля Jool и DKMS, согласования версий модуля/CLI.
- Нет EAMT и списка IPv4-подсетей клиентов для source mapping.
- Нет маршрута клиентского технического /96 на хосте.
- Нет динамической перезагрузки TAYGA.
- Установка назначения — обычное правило DNAT6.

Остаются TUN, NET_ADMIN/MKNOD, IPv6 forwarding, IPv6-канал контейнер—хост,
NAT66 и IPv4 default route через docker_gwbridge. TUN — штатный драйвер Linux;
его доступность всё равно является предпосылкой.

Прототип использует NAT66 внутри контейнера. Переключатель N46_NAT66=0
отключает только внешний host NAT66 для теста маршрутизируемой ULA-сети;
это не режим сохранения индивидуальных публичных IPv6 всех VPN-клиентов.

## Воспроизведение

Только отдельная локальная Ubuntu VM. Скрипт меняет её маршруты, Docker-сети
и firewall. Он проверяет marker, отсутствие ресурсов с именами n46-* и отсутствие
загруженного jool_siit. Не запускать на production, не запускать параллельно.

С Lima (в текущей сессии бинарник находится в tmp/nat46-vm/lima/bin/limactl):

```sh
limactl start --name nat46-tayga --tty=false scripts-diag/nat46-tayga/lima.yaml
limactl copy scripts-diag/nat46-tayga/provision-vm.sh nat46-tayga:/tmp/provision-vm.sh
limactl shell nat46-tayga bash /tmp/provision-vm.sh --disposable-vm
limactl shell nat46-tayga mkdir -p /tmp/nat46-tayga
limactl copy scripts-diag/nat46-tayga/Dockerfile \
  scripts-diag/nat46-tayga/packet-test.py scripts-diag/nat46-tayga/quic-test.py \
  nat46-tayga:/tmp/nat46-tayga/
limactl shell nat46-tayga sudo docker build -t nat46-tayga-test:local /tmp/nat46-tayga
limactl shell nat46-tayga sudo env N46_QUIC=1 python3 /tmp/nat46-tayga/packet-test.py
```

Для Swarm: один раз выполнить `docker swarm init --advertise-addr VM_IPV4`
внутри VM, указав реальный адрес её интерфейса. Затем:

```sh
limactl shell nat46-tayga sudo env N46_SWARM=1 N46_QUIC=1 \
  python3 /tmp/nat46-tayga/packet-test.py
```

Forwarding задаётся через sysctls при создании задачи Swarm. Запись sysctl из
самого контейнера не сработала из-за read-only /proc/sys; утилита при этом
вернула код 0. Стенд проверяет фактические значения, а не только exit status.
TUN в проверенном Swarm создаётся с NET_ADMIN/MKNOD без privileged и без
дополнительного device mount.

Ресурсы стенда удаляются в finally. Изменение host IPv6 forwarding остаётся
до остановки VM. N46_KEEP=1 оставляет ресурсы для диагностики; перед повтором
их надо удалить. После тестов: `limactl stop nat46-tayga`.

## Границы эксперимента

Проверены отдельные Docker-контейнеры и одноузловой Swarm, контролируемый
IPv6-сервер. Производительность, реальные VPN-туннели, два узла Swarm,
внешний IPv6 провайдера и DNS-интеграция не проверены этим стендом.
NAT66 в два этапа создаёт дополнительное состояние conntrack; производительность
с Jool не сравнивалась. Следующий этап — интеграция backend DNAT6 с dnsmap,
сохранение/восстановление и повтор DNS-тестов перед выбором production-варианта.
