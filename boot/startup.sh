#!/bin/sh

export PATH="/usr/bin:/usr/local/bin:/bin:/root/bin:/sbin:/usr/sbin:/usr/local/sbin"
export HOME="/root"

/init -e 1 /bin/vsockexec -o 109 -e 109 /bin/gcs -v4 -log-format json -loglevel debug