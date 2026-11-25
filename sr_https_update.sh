#!/bin/bash

./sr_git_pull.sh

docker compose up -d --no-deps --force-recreate --build https
docker compose up -d --no-deps --force-recreate --build dashboard
docker compose up -d --no-deps --force-recreate --build authelia

