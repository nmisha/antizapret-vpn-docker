#!/bin/bash

docker compose down --remove-orphans

docker system prune -af


#ls -la ./services/proxy/files/
#chmod +x ./services/proxy/files/init.sh

#git pull
./sr_git_pull.sh

#make exec for sh
./sr_make_executable.sh

#down & rebuld & up
./sr_update.sh



