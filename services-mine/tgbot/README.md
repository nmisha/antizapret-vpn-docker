# tgbot

Telegram-бот для пользователей и админов antizapret: VPN-профили (WireGuard, Amnezia WireGuard,
OpenVPN), MTProto-прокси (telemt), списки доменов, DNS Guard, учётные записи, поддержка.
Пользователи и роли — в `users.json`, управление из бота: `/users_admin`.

## Справка `/help`

### Как строится

- Каждая команда регистрируется через `CommandRegistry.Command(CommandSpec{...})`
  (`internal/bot/command_registry.go`) с полями `Cmd`, `Args`, `Desc`, `Section`, `NeedAny`, `Hidden`.
  Порядок регистрации задаёт `internal/modules/register_all.go`.
- Справка показывает только доступные пользователю команды: без `NeedAny` — всем, иначе — при
  наличии хотя бы одной роли из списка. `Admin` включает все роли. Команды с `Hidden: true`
  (старые синонимы) в справку не попадают, но работают.
- Разделы (`Section`) задаются только константами `sec…` из `internal/bot/help_sections.go`.
  Там же `helpGroups`: какие разделы входят в какую группу справки.
- Без `TELEMT_TOKEN` команды MTProto не регистрируются и в справке отсутствуют.
- Меню команд Telegram (кнопка «/» в клиенте, `setMyCommands`) бот не задаёт.

### Что видит пользователь

| Команда | Ответ |
|---|---|
| `/help`, `/start` при ≤ 20 доступных команд | весь список одним сообщением, по разделам |
| `/help`, `/start` при > 20 команд (админ) | оглавление: группы кнопками с числом команд |
| кнопка группы | список группы в том же сообщении, кнопка «← Разделы» |
| `/help <группа>` | список группы без кнопок оглавления (`/help wg`) |
| `/help all` | весь список, частями по 3500 символов |

`/start` игнорирует параметр deep link и показывает то же, что `/help`. На экране группы внутри каждого
раздела сначала команды без аргументов (обычно меню «UI: …»), затем команды с аргументами, каждая часть
по алфавиту. В полном списке (короткая справка или `/help all`) команды внутри разделов идут по алфавиту.

Проверки: `HelpProblems` (вызывается при старте из `ValidateHelpCoverage`, пишет в лог, и в тесте
`internal/modules/help_test.go`) — раздел без группы и экран админа длиннее 4000 символов.

### Группы

| Ключ | Группа | Разделы | Команд у админа |
|---|---|---|---|
| `start` | Основное | Быстрый старт | 4 |
| `wg` | WireGuard | WireGuard, Админ: WireGuard | 12 |
| `awg` | Amnezia WireGuard | Amnezia WireGuard, Админ: Amnezia WireGuard | 12 |
| `ovpn` | OpenVPN | OpenVPN, Админ: OpenVPN | 13 |
| `mtproto` | MTProto | MTProto, Админ: MTProto | 2 |
| `dns` | Домены и DNS | Домены, DNS Guard, Сервисы, Админ: сервисы, Сеть | 13 |
| `accounts` | Аккаунты | Аккаунты, Админ: аккаунты | 7 |
| `users` | Админ: пользователи | Админ: пользователи и роли, Админ: сообщения | 15 |
| `bot` | Админ: бот | Админ: настройки и логи | 12 |

Всего зарегистрировано 97 команд, из них 7 скрытых синонимов; админ видит 90.

### Команды

Как их видит админ в группах; обычный пользователь видит подмножество по ролям.

#### Основное (`/help start`)

| Команда | Роли | Описание |
|---|---|---|
| `/start` | все | показать справку |
| `/help [раздел\|all]` | все | показать эту справку |
| `/roles [name]` | все | показать роли (свои; Admin может смотреть чужие) |
| `/support [сообщение]` | все | написать в поддержку (без аргумента откроет UI) |

#### WireGuard (`/help wg`)

*WireGuard*

| Команда | Роли | Описание |
|---|---|---|
| `/wg_profiles` | WgUserControl | UI: мои WireGuard профили |
| `/wg_stats` | WgStats | статистика WG (для своего пользователя) |
| `/wgstats` | WgStats | статистика WG (legacy alias) *(скрыта)* |

*Админ: WireGuard*

| Команда | Роли | Описание |
|---|---|---|
| `/wg_profiles_admin` | Admin | UI: WireGuard профили (admin) |
| `/wg_stats_admin` | Admin | статистика WG (admin) |
| `/wgstats_admin` | Admin | статистика WG (legacy alias) *(скрыта)* |
| `/wg_conf <profile_name_or_prefix>` | Admin | получить конфиг |
| `/wg_del <profile_name_or_prefix>` | Admin | удалить профиль |
| `/wg_disable <profile_name_or_prefix>` | Admin | выключить профиль |
| `/wg_enable <profile_name_or_prefix>` | Admin | включить профиль |
| `/wg_qr <profile_name_or_prefix>` | Admin | получить QR |
| `/wg_rename <old_name_or_prefix> <new_name>` | Admin | переименовать профиль |
| `/wg_reset_score <profile_name_or_prefix>` | Admin | сбросить risk score DNS Guard |
| `/wg_score <profile_name_or_prefix>` | Admin | получить risk score профиля |

#### Amnezia WireGuard (`/help awg`)

*Amnezia WireGuard*

| Команда | Роли | Описание |
|---|---|---|
| `/awg_profiles` | AwgUserControl | UI: мои Amnezia WireGuard профили |
| `/awg_stats` | AwgStats | статистика AWG по своим профилям |
| `/awgstats` | AwgStats | статистика AWG (legacy alias) *(скрыта)* |

*Админ: Amnezia WireGuard*

| Команда | Роли | Описание |
|---|---|---|
| `/awg_profiles_admin` | Admin | UI: Amnezia WireGuard профили (admin) |
| `/awg_stats_admin` | Admin | статистика AWG (admin) |
| `/awgstats_admin` | Admin | статистика AWG (legacy alias) *(скрыта)* |
| `/awg_conf <profile_name_or_prefix>` | Admin | получить конфиг |
| `/awg_del <profile_name_or_prefix>` | Admin | удалить профиль |
| `/awg_disable <profile_name_or_prefix>` | Admin | выключить профиль |
| `/awg_enable <profile_name_or_prefix>` | Admin | включить профиль |
| `/awg_qr <profile_name_or_prefix>` | Admin | получить QR |
| `/awg_rename <old_name_or_prefix> <new_name>` | Admin | переименовать профиль |
| `/awg_reset_score <profile_name_or_prefix>` | Admin | сбросить risk score DNS Guard |
| `/awg_score <profile_name_or_prefix>` | Admin | получить risk score профиля |

#### OpenVPN (`/help ovpn`)

*OpenVPN*

| Команда | Роли | Описание |
|---|---|---|
| `/ovpn_profiles` | OvpnUserControl | UI: мои OpenVPN профили |
| `/ovpn_stats` | OvpnStats | статистика OpenVPN по своим профилям |
| `/ovpnstats` | OvpnStats | статистика OpenVPN (legacy alias) *(скрыта)* |

*Админ: OpenVPN*

| Команда | Роли | Описание |
|---|---|---|
| `/ovpn_profiles_admin` | Admin | UI: OpenVPN профили (admin) |
| `/ovpn_stats_admin` | Admin | статистика OpenVPN (admin) |
| `/ovpnstats_admin` | Admin | статистика OpenVPN (legacy alias) *(скрыта)* |
| `/ovpn_burn <profile_name_or_prefix>` | Admin | удалить уже отозванный сертификат |
| `/ovpn_conf <profile_name_or_prefix>` | Admin | получить конфиг профиля |
| `/ovpn_create <certificate_name>` | Admin | создать новый сертификат |
| `/ovpn_reset_score <profile_name_or_prefix>` | Admin | сбросить risk score DNS Guard |
| `/ovpn_restart <profile_name_or_prefix>` | Admin | перезапустить OpenVPN server |
| `/ovpn_restart_container <profile_name_or_prefix>` | Admin | перезапустить OpenVPN container |
| `/ovpn_revoke <profile_name_or_prefix>` | Admin | отозвать сертификат |
| `/ovpn_score <profile_name_or_prefix>` | Admin | получить risk score профиля |
| `/ovpn_stats_profile <profile_name_or_prefix>` | Admin | статистика по профилю |

#### MTProto (`/help mtproto`)

*MTProto*

| Команда | Роли | Описание |
|---|---|---|
| `/mtproto` | MtprotoUserControl | UI: мои MTProto прокси для Telegram |

*Админ: MTProto*

| Команда | Роли | Описание |
|---|---|---|
| `/mtproto_admin` | Admin | UI: MTProto прокси (admin) |

#### Домены и DNS (`/help dns`)

*Домены*

| Команда | Роли | Описание |
|---|---|---|
| `/export` | DomainManager | экспортировать все домены |
| `/list` | DomainEditor | показать домены в твоей секции |
| `/add <domain>` | DomainEditor | добавить домен в твою секцию |
| `/del <domain>` | DomainEditor | удалить домен из твоей секции |

*DNS Guard*

| Команда | Роли | Описание |
|---|---|---|
| `/guard_notify_off` | все | отключить DNS Guard уведомления |
| `/guard_notify_on` | все | включить DNS Guard уведомления |
| `/guard_notify_status` | все | показать статус DNS Guard уведомлений |
| `/guard_notme_notify_off` | Admin | отключить admin-уведомления DNS Guard по другим пользователям |
| `/guard_notme_notify_on` | Admin | включить admin-уведомления DNS Guard по другим пользователям |

*Сервисы*

| Команда | Роли | Описание |
|---|---|---|
| `/agh_update_lists` | DomainEditor, ServiceManager | обновить списки AdGuard Home (filters) |
| `/sr_agh_update_lists` | DomainEditor, ServiceManager | legacy alias *(скрыта)* |

*Админ: сервисы*

| Команда | Роли | Описание |
|---|---|---|
| `/agh_last_update` | Admin | дата и время последнего обновления списков AdGuard Home |

*Сеть*

| Команда | Роли | Описание |
|---|---|---|
| `/net_get_iperf3` | NetUser | готовые iperf3 команды для near/far node |
| `/net_dns_resolve [domain]` | NetUser | DNS resolve через заданный DNS-сервер |

#### Аккаунты (`/help accounts`)

*Аккаунты*

| Команда | Роли | Описание |
|---|---|---|
| `/accounts` | AiUser | UI: получить учётные записи |

*Админ: аккаунты*

| Команда | Роли | Описание |
|---|---|---|
| `/acc_list` | Admin | список учётных записей |
| `/accounts_admin` | Admin | UI: управление учётными записями |
| `/acc_add <name> <login> <password>` | Admin | добавить/обновить учётную запись |
| `/acc_del <name>` | Admin | удалить учётную запись |
| `/acc_rename <old_name> <new_name>` | Admin | переименовать учётную запись |
| `/acc_set <name> <login> <password>` | Admin | обновить логин/пароль учётной записи |

#### Админ: пользователи (`/help users`)

*Админ: пользователи и роли*

| Команда | Роли | Описание |
|---|---|---|
| `/roles_catalog` | Admin | все роли с описанием |
| `/users` | Admin | список пользователей и ролей |
| `/users_admin` | Admin | UI: пользователи бота, роли, префиксы профилей, заявки |
| `/grant <name> <role>` | Admin | выдать роль пользователю |
| `/prefix_add <name> <wg\|ovpn\|mtproto> <prefix>` | Admin | добавить префикс профилей пользователю |
| `/prefix_del <name> <wg\|ovpn\|mtproto> <prefix>` | Admin | убрать префикс профилей у пользователя |
| `/rename <old_name> <new_name>` | Admin | переименовать пользователя |
| `/revoke <name> <role>` | Admin | снять роль с пользователя |
| `/user_add <name> <tg_id>` | Admin | добавить пользователя |
| `/user_del <name>` | Admin | удалить пользователя |
| `/user_disable <name>` | Admin | отключить пользователя от бота и уведомлений |
| `/user_enable <name>` | Admin | снова включить пользователя |

*Админ: сообщения*

| Команда | Роли | Описание |
|---|---|---|
| `/adminmsg` | Admin | UI: написать пользователю |
| `/broadcast <text>` | Admin | отправить всем (без аргумента откроет UI) |
| `/message <name> <text>` | Admin | написать пользователю (без UI) |

#### Админ: бот (`/help bot`)

| Команда | Роли | Описание |
|---|---|---|
| `/bot_off` | Admin | выключить ответы бота |
| `/bot_on` | Admin | включить ответы бота |
| `/log` | Admin | показать последние строки лога |
| `/log_clear` | Admin | очистить лог |
| `/log_cmd_off` | Admin | выключить логирование команд |
| `/log_cmd_on` | Admin | включить логирование команд |
| `/log_errors_off` | Admin | выключить логирование ошибок |
| `/log_errors_on` | Admin | включить логирование ошибок |
| `/log_off` | Admin | выключить логирование (оба) |
| `/log_on` | Admin | включить логирование (оба) |
| `/settings` | Admin | настройки бота (логирование, отключение ответов) |
| `/log_tail <N>` | Admin | настроить сколько строк показывает /log |
