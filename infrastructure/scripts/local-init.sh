#!/bin/sh
set -eu
umask 077
mkdir -p /runtime
if [ ! -s /runtime/env ]; then
  random() { od -An -N32 -tx1 /dev/urandom | tr -d ' \n'; }
  {
    printf 'ENCRYPTION_KEY=%s\n' "$(random)"
    printf 'SETUP_TOKEN=%s\n' "$(random)"
  } > /runtime/env.tmp
  chown 10001:10001 /runtime/env.tmp
  mv /runtime/env.tmp /runtime/env
fi
