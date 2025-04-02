#!/bin/bash

eval "$(ssh-agent -s)"
ssh-add /root/.ssh/github_az

git pull
