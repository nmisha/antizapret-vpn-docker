#!/bin/bash

git pull
docker compose pull
docker compose build
docker compose down --remove-orphans && docker compose up -d --remove-orphans

#docker system prune -f