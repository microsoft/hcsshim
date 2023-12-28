#!/bin/sh

export PATH="/usr/bin:/usr/local/bin:/bin:/root/bin:/sbin:/usr/sbin:/usr/local/sbin"
export HOME="/root"

/bin/vsockexec -o 2056 -e 2056 echo Running startup_v2056.sh
/bin/vsockexec -o 2056 -e 2056 date

/bin/vsockexec -o 2056 -e 2056 echo /init -e 1 /bin/vsockexec -o 2056 -e 109 /bin/gcs -v4 -log-format text -loglevel debug -logfile /tmp/gcs.log
/init -e 1 /bin/vsockexec -o 2056 -e 109 /bin/gcs -v4 -log-format text -loglevel debug -logfile /tmp/gcs.log

/bin/vsockexec -o 2056 -e 2056 echo ls -l /dev/dm*
/bin/vsockexec -o 2056 -e 2056 ls -l /dev/dm*
/bin/vsockexec -o 2056 -e 2056 echo ls -l /dev/mapper
/bin/vsockexec -o 2056 -e 2056 ls -l /dev/mapper
/bin/vsockexec -o 2056 -e 2056 echo ls -l /dev/mapper
/bin/vsockexec -o 2056 -e 2056 ls -l /dev/mapper

#/bin/vsockexec -o 2056 -e 2056 /bin/snp-report

# need init to have run before top shows much
/bin/vsockexec -o 2056 -e 2056 top -n 1

/bin/vsockexec -o 2056 -e 2056 echo tmp
/bin/vsockexec -o 2056 -e 2056 ls -la /tmp

/bin/vsockexec -o 2056 -e 2056 /bin/dmesg

sleep 1
/bin/vsockexec -o 2056 -e 2056 echo Thats all folks...




