#!/bin/bash

./fix_git.sh

docker compose up -d --no-deps --force-recreate --build proxy
docker compose up -d --no-deps --force-recreate --build dashboard
docker compose up -d --no-deps --force-recreate --build authelia

