# 2FAuth

Сервис использует официальный образ `2fauth/2fauth` и хранит данные в
`config-mine/2fauth`. Веб-интерфейс доступен через `https` по адресу
`https://twof.auth.vpn.example.com`, внутренний адрес — `http://2fauth:8000`.
Доменный маршрут и проверку Authelia настраивает генератор HTTPS через
`PROXY_VHOST_1`; [порядок переноса и сборки образов](../../services/https/README.md).

В локальном Swarm override включён `reverse-proxy-guard`: группа Authelia
`twofauth_shared` входит в общий аккаунт `chatgpt`. Остальные разрешённые
пользователи сопоставляются по личному логину. Caddy и 2FAuth используют отдельную
сеть `twofauth-auth`; 2FAuth доверяет только диапазону `10.43.42.0/24`.
После применения этого режима собственный WebAuthn и Personal Access Tokens
2FAuth не поддерживаются. [Настройка общего входа](../authelia/README.md).

При каждом запуске стартовый скрипт устанавливает владельца `1000:1000`
для `/2fauth` и его содержимого, а для самого каталога — права `700`.
Затем штатный entrypoint запускается от пользователя `1000:1000`.
Root используется только для подготовки; приложение работает без root.
Собственная сборка образа не нужна.

Это исправляет права и после пересоздания каталога на узле Swarm.
Сам bind-каталог должен существовать до запуска задачи: если он полностью
удалён, создайте его на узле с меткой `location=world` по абсолютному пути,
указанному в mount сервиса. Swarm может отклонить отсутствующий bind source
ещё до запуска скрипта. Вручную выполнять chown/chmod не нужно.
Удаление каталога удаляет и данные; исправление прав их не восстанавливает.

В корневом override подключите сервис через `extends` и задайте напрямую
`APP_URL` (полный внешний HTTPS-адрес) и `APP_KEY` (постоянный уникальный ключ)
в `services.2fauth.environment`. Пример подключения находится в
`config-docker-swarm/docker-compose.override.yml` и предназначен для копирования
в корень проекта на сервере.

Генерация ключа для новой установки:

```sh
docker run --rm --entrypoint /usr/bin/php 2fauth/2fauth:latest artisan key:generate --show
```

Сохраните ключ вместе с резервной копией данных; не меняйте его при обновлениях.

После обновления Compose-файла разверните стек штатным скриптом на менеджере:

```sh
sh sr_swarm_start.sh
docker service logs --since 2m --tail 100 antizapret_2fauth
```

## Замена локального пароля пользователя

Пароли 2FAuth хранятся в виде хешей: исходный пароль прочитать нельзя,
но можно установить новый. Это не меняет пароль Authelia, секреты 2FA
и режим входа `reverse-proxy-guard`.

Команды выполняются на узле Swarm, где запущен контейнер 2FAuth.
Найти узел можно на менеджере:

```sh
docker service ps antizapret_2fauth --filter desired-state=running
```

На найденном узле откройте Laravel Tinker:

```sh
docker exec -it \
  "$(docker ps -q --filter name=antizapret_2fauth)" \
  php /srv/artisan tinker
```

Посмотрите логины пользователей, не выводя хеши и другие поля:

```php
\App\Models\User::query()->get(['id', 'name']);
```

Если готового хеша нет, сгенерируйте его средствами приложения. Следующую
команду вставьте в Tinker одной строкой, затем введите новый пароль
в скрытом приглашении. Пароль не включается в историю команд, а результатом
будет только хеш:

```php
$hash = \Illuminate\Support\Facades\Hash::make((new \Symfony\Component\Console\Helper\QuestionHelper())->ask(new \Symfony\Component\Console\Input\ArgvInput(), new \Symfony\Component\Console\Output\ConsoleOutput(), (new \Symfony\Component\Console\Question\Question('Новый пароль: '))->setHidden(true)->setHiddenFallback(false)));
```

Если хеш уже сгенерирован, вместо предыдущей команды присвойте его переменной.
Замените `ВСТАВЬТЕ_ПОЛНЫЙ_ХЕШ` своим значением; одинарные кавычки сохраняют
символы `$` в bcrypt-хеше:

```php
$hash = 'ВСТАВЬТЕ_ПОЛНЫЙ_ХЕШ';
```

Для замены пароля **одного пользователя**, например `chatgpt`:

```php
$user = \App\Models\User::where('name', 'chatgpt')->firstOrFail();
$user->password = $hash;
$user->save();
$user->refresh()->getRawOriginal('password') === $hash;
```

Последние две команды должны вернуть `true`.

Если нужно намеренно установить один пароль **всем пользователям 2FAuth**,
вместо блока для одного пользователя выполните:

```php
\Illuminate\Support\Facades\DB::table('users')->update(['password' => $hash]);
\Illuminate\Support\Facades\DB::table('users')->where('password', $hash)->count() === \Illuminate\Support\Facades\DB::table('users')->count();
```

Первая команда вернёт число обновлённых строк, вторая должна вернуть `true`.
Для выхода из Tinker выполните `exit`. Перезапуск 2FAuth не требуется.
Смена локального пароля сама по себе не включает обычную форму входа
при активном `reverse-proxy-guard`.

[Документация Docker-образа](https://docs.2fauth.app/getting-started/installation/docker/docker-cli/).
