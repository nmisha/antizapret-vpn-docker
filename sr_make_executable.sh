
#!/bin/sh

TARGET_DIR="${1:-.}"  # если путь не указан, берём текущую папку

# find "$TARGET_DIR" -type f -name "*.sh" -exec chmod +x {} \;
find "$TARGET_DIR" -type f -name "*.sh" -print -exec chmod +x {} \;

echo "Все .sh скрипты в '$TARGET_DIR' сделаны исполняемыми."

