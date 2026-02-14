#!/bin/bash

eval "$(ssh-agent -s)"
ssh-add /root/.ssh/github_az

git pull

#chmod +x ./services/proxy/files/init.sh
#chmod +x ./services/dashboard/files/init.sh

