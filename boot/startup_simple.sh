#!/bin/sh

export PATH="/usr/bin:/usr/local/bin:/bin:/root/bin:/sbin:/usr/sbin:/usr/local/sbin"
export HOME="/root"

/bin/vsockexec -o 2056 -e 2056 echo Running startup_simple.sh
/bin/vsockexec -o 2056 -e 2056 date

/bin/vsockexec -o 2056 -e 2056 echo /init -e 1 /bin/vsockexec -o 2056 -e 109 /bin/gcs -v4 -log-format text -loglevel debug -logfile /tmp/gcs.log
/init -e 1 /bin/vsockexec -o 2056 -e 109 /bin/gcs -v4 -log-format text -loglevel debug -logfile /tmp/gcs.log

/bin/vsockexec -o 2056 -e 2056 echo dmesg
/bin/vsockexec -o 2056 -e 2056 dmesg

/bin/vsockexec -o 2056 -e 2056 echo sleeping 2
/bin/vsockexec -o 2056 -e 2056 sleep 2

/bin/vsockexec -o 2056 -e 2056 ls -Rl /dev/se*

