#!/bin/bash

eval "$(ssh-agent -s)"
ssh-add /root/.ssh/github_az

git reset --hard HEAD
git pull --rebase

# git pull

# ./sr_make_executable.sh

#chmod +x ./services/proxy/files/init.sh
#chmod +x ./services/dashboard/files/init.sh

