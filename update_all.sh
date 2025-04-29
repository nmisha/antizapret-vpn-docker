#!/bin/bash

docker compose down --remove-orphans

docker system prune -af


./fix_git.sh

ls -la ./services/proxy/files/
chmod +x ./services/proxy/files/init.sh

./update.sh



