# Authelia: WebAuthn и passkeys

Рабочая конфигурация находится в `config-mine/authelia/config/configuration.yml`.
Этот каталог исключён из Git, поэтому конфигурацию нужно отдельно переносить
на узел Authelia. Настройки ниже проверены командой `authelia validate-config`
на локальных образах 4.39.16 и 4.39.20.

```yaml
webauthn:
  display_name: MILAN
  disable: false
  enable_passkey_login: true
  timeout: 120 seconds
  attestation_conveyance_preference: none
  selection_criteria:
    attachment: ''
    discoverability: preferred
    user_verification: preferred
  filtering:
    prohibit_backup_eligibility: false
```

Пустой `attachment` позволяет выбирать встроенный аутентификатор, менеджер
passkeys или внешний FIDO2-ключ. `preferred` сохраняет совместимость с ключами,
которые не поддерживают discoverable credentials или проверку PIN/биометрией:
такие ключи могут служить вторым фактором, но не обязательно подходят для
беспарольного входа. Для входа без пароля регистрируйте именно passkey.
Синхронизируемые ключи не запрещены. Аттестация модели устройства не запрашивается.

## Вход с телефона на компьютере

Cross-device authentication (hybrid transport) предлагает браузер: в диалоге
passkey выберите другое устройство, отсканируйте QR-код телефоном с ключом и
подтвердите вход. Обычно требуются Bluetooth на обоих устройствах для проверки
близости, доступ в Интернет и совместимые браузер/ОС. Authelia не генерирует этот
QR-код и не управляет синхронизацией ключей между менеджерами паролей. Отдельного
параметра `cross_passkey` нет; `attachment: cross-platform` не является его заменой
и ограничил бы выбор встроенных аутентификаторов.

## Регистрация и применение

1. Перенесите конфигурацию на узел и перезапустите Authelia. Обновление файла
   bind mount само по себе не гарантирует перезапуск задачи Swarm.
2. Откройте `https://auth.nope.jo3.org` в отдельной вкладке с действительным
   доверенным HTTPS-сертификатом. Войдите паролем и зарегистрируйте passkey
   в настройках аккаунта. На странице входа затем можно использовать passkey.
3. Сейчас настроен `notifier.filesystem`, а не SMTP. Подтверждение регистрации
   ищите в `config-mine/authelia/config/notification.txt` на узле Authelia;
   email автоматически не отправляется. Для самостоятельной регистрации
   другими пользователями потребуется настроить SMTP.
4. Проверьте вход с локальным ключом и с телефона через QR. Ключ, созданный для
   прежнего адреса портала, может потребовать повторной регистрации на новом
   домене. Не удаляйте рабочий способ входа до проверки нового ключа.

Текущие правила `one_factor` позволяют вход паролем или passkey. WebAuthn
не делает второй фактор обязательным автоматически. Для `two_factor` Authelia
по умолчанию потребует пароль после входа по passkey. Экспериментальный параметр
`experimental_enable_passkey_uv_two_factors` не включён. Существующие TOTP,
пароли и сроки сессии сохранены; `remember_me` уже включён.

## Связь пользователей с 2FAuth

В локальном Swarm override включена авторизация 2FAuth через Authelia:

```yaml
AUTHENTICATION_GUARD: reverse-proxy-guard
AUTH_PROXY_HEADER_FOR_USER: HTTP_REMOTE_USER
AUTH_PROXY_HEADER_FOR_EMAIL: HTTP_REMOTE_EMAIL
TRUSTED_PROXIES: 10.43.42.0/24
```

`HTTP_REMOTE_USER` и `HTTP_REMOTE_EMAIL` — имена PHP server variables для
заголовков `Remote-User` и `Remote-Email`. Проверенный локальный образ 2FAuth
читает их через `request()->server()` и требует доверенный адрес источника.

Группа `twofauth_shared` сопоставляется с существующим аккаунтом 2FAuth `chatgpt`.
Участники: `mi`, `kablag`, `dkreynes`. Они входят в Authelia под
своими логинами, а Caddy после успешной проверки заменяет `Remote-User` на
`chatgpt`. Email и отображаемое имя участника удаляются из запроса к 2FAuth,
поэтому общий профиль не меняется при входе разных пользователей.

Правило Authelia разрешает группе доступ только к `twof.auth.nope.jo3.org`.
Остальные разрешённые пользователи используют личное сопоставление. Членство
в новой группе само по себе не открывает другие приложения и не делает новых
пользователей администраторами Authelia. Внутри общего аккаунта 2FAuth участники
получают все права самого аккаунта `chatgpt`.

Перед применением на сервере:

- Сопоставьте логин Authelia (ключ пользователя в `users_database.yml`, который
  передаётся в `Remote-User`) с полем `name` нужного аккаунта 2FAuth. Проверенный
  `RemoteUserProvider` ищет по `name`, а не по email или отображаемому имени
  Authelia. При совпадении используется существующий аккаунт с его данными;
  иначе создаётся новый аккаунт. Совпадение email не объединяет аккаунты.
- Проверьте отсутствие пересечения подсети `10.43.42.0/24` на узлах. В override
  только Caddy и 2FAuth подключены к сети `twofauth-auth`; 2FAuth больше не
  подключён к общей сети и не публикует порт 8000. Права администратора Docker
  по-прежнему позволяют подключать контейнеры к этой сети.
- Соберите и опубликуйте новый образ HTTPS с обновлённым генератором и используйте
  его новый тег в стеке. Порядок обработки закреплён блоком `route`: удаление
  входных `Remote-*`/`Remote_*`, затем `forward_auth`, затем сопоставление группы.
- Перенесите `configuration.yml` и `users_database.yml` на узел Authelia;
  конфигурации `config-mine` и `config-docker-swarm` исключены из Git. Начальные
  пароли новых пользователей сохранены в
  `config-mine/authelia/new-users-credentials.txt`; передавайте их отдельно
  приватным способом и не копируйте этот файл в каталог `/config` контейнера.
- Сделайте резервную копию данных и сохраните `APP_KEY`. Проверьте каждый
  существующий аккаунт до окончательного переключения.

После переключения участники группы входят паролем или passkey в Authelia и
попадают в общий аккаунт 2FAuth. Пароли новых пользователей случайные; изменение
пароля пользователем сейчас отключено в конфигурации Authelia. Базы, пароли и
passkeys между приложениями не синхронизируются.
Согласно документации 2FAuth, собственные WebAuthn и Personal Access Tokens
не поддерживаются в режиме `reverse-proxy-guard`.

Источники:

- [Настройки WebAuthn Authelia](https://www.authelia.com/configuration/second-factor/webauthn/)
- [Security Key и Passkeys](https://www.authelia.com/overview/authentication/security-key/)
- [Cross-device passkeys](https://developers.google.com/identity/passkeys/use-cases)
- [2FAuth auth proxy](https://docs.2fauth.app/security/authentication/auth-proxy/)
