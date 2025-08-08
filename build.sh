#!/usr/bin/env bash
set -euo pipefail
IMAGE="${IMAGE:-forum}"
docker build -t "$IMAGE:latest" .
docker run --rm -d \
  -p 8080:8080 \
  -v forum_data:/app/data \
  --name forum \
  "$IMAGE:latest"
echo "Up at http://localhost:8080"
