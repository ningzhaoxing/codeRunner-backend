#!/bin/sh
set -e

# If docker.sock is mounted (worker/client mode), match host docker group GID
# so the unprivileged runner user can access it.
if [ -S /var/run/docker.sock ]; then
  DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)

  # Create a group with matching GID if none exists yet
  if ! awk -F: -v gid="$DOCKER_GID" '$3 == gid' /etc/group | grep -q .; then
    addgroup -g "$DOCKER_GID" docker
  fi

  # Resolve the group name for that GID (may already exist under another name)
  GROUP_NAME=$(awk -F: -v gid="$DOCKER_GID" '$3 == gid {print $1; exit}' /etc/group)
  addgroup runner "$GROUP_NAME" 2>/dev/null || true
fi

# Drop privileges and exec the app
exec su-exec runner /app/code-runner "$@"
