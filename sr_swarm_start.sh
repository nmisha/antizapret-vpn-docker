#!/bin/sh
set -eu
umask 077

# Этапы разделены временными файлами, чтобы set -e остановил скрипт при ошибке
# compose или конвертера до deploy. В обычном /bin/sh ошибка в начале pipeline
# может остаться незамеченной. Это дополнительная защита, не требование tmpfs.
# Итоговая конфигурация может содержать секреты: umask 077 ограничивает доступ,
# а trap удаляет временные файлы при завершении.
work_dir=$(mktemp -d)
trap 'rm -rf -- "$work_dir"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker compose --env-file compose.swarm.env config > "$work_dir/compose.yml"
docker run --pull always --rm -i xtrime/antizapret-vpn:6 compose2swarm \
  < "$work_dir/compose.yml" > "$work_dir/converted.yml"

# Для Swarm tmpfs задан через volumes в длинной форме. Текущий compose2swarm
# из xtrime/antizapret-vpn:6 превращает размер в строку: size: "4194304".
# Docker stack требует целое число и без исправления отвергает конфигурацию.
# Ниже обход ошибки без пересборки образа: снимаем кавычки только у полей size
# с десятичными цифрами. Значения с единицами измерения не изменяем.
# Основное исправление следует внести в сериализацию compose2swarm; этот обход
# можно удалить после перехода на образ конвертера, сохраняющий числовой size.
sed -E 's/^([[:space:]]+size: )"([0-9]+)"$/\1\2/' \
  "$work_dir/converted.yml" > "$work_dir/stack.yml"
docker stack config -c "$work_dir/stack.yml" > /dev/null
docker stack deploy --prune -c "$work_dir/stack.yml" antizapret

# OLD =============================
# #!/bin/sh

# # docker compose config | docker run --rm -i xtrime/antizapret-vpn:6 compose2swarm | docker stack deploy --prune -c - antizapret

# docker compose --env-file compose.swarm.env config | docker run --pull always --rm -i xtrime/antizapret-vpn:6 compose2swarm | docker stack deploy --prune -c - antizapret




# # !/bin/sh

# # set -eu
# # umask 077
# # # Stop at the first failed stage. Rendered files may contain secrets.
# # work_dir=$(mktemp -d)
# # trap 'rm -rf -- "$work_dir"' EXIT
# # trap 'exit 130' INT
# # trap 'exit 143' TERM

# # docker compose --env-file compose.swarm.env config > "$work_dir/compose.yml"
# # docker run --pull always --rm -i xtrime/antizapret-vpn:6 compose2swarm \
# #   < "$work_dir/compose.yml" > "$work_dir/stack.yml"
# # docker stack config -c "$work_dir/stack.yml" > /dev/null
# # docker stack deploy --prune -c "$work_dir/stack.yml" antizapret
