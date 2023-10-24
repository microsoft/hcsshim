#!/bin/sh

export PATH="/usr/bin:/usr/local/bin:/bin:/root/bin:/sbin:/usr/sbin:/usr/local/sbin"
export HOME="/root"

/bin/vsockexec -o 2056 echo STARTUP_2
/bin/vsockexec -o 2056 echo Sep 25th 2023
/bin/vsockexec -o 2056 -e 2056 date


/bin/vsockexec -o 2056 echo /init -e 1 /bin/vsockexec -e 109 /bin/gcs -v4 -log-format json -loglevel debug
/init -e 1 /bin/vsockexec -o 2056 -e 109 /bin/gcs -v4 -log-format text -loglevel debug -logfile /tmp/gcs.log



/bin/vsockexec -o 2056 ls -Rl /dev/
/bin/vsockexec -o 2056 dmesg
/bin/vsockexec -o 2056 ls -Rl /dev/mapper
/bin/vsockexec -o 2056 lsblk
/bin/vsockexec -o 2056 echo STARTUP_2


# /init2 -e 1 /bin/vsockexec -o 109 -e 109 /bin/gcs -v4 -log-format json -loglevel debug
