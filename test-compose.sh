#!/bin/bash
DOCKER_HOST=ssh://root@65.21.48.1 docker compose -f internal/files/docker-compose.yml up -d --build
