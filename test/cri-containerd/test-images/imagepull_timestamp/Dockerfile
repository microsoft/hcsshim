# This Dockerfile builds a docker image based on top of the nanoserver image that is
# used during the pull image tests in hcsshim. Tests in hcsshim/test/cri-containerd/pullimage_test.go
# are directly dependent on the directory structure of this image. If anything is changed
# in this Dockerfile, please make sure to update the tests too.

# As of now this image is built with:
# `docker build -t cplatpublic.azurecr.io/timestamp:latest .`
# And this image is pushed to a private repo with:
# `docker push cplatpublic.azurecr.io/timestamp:latest`

# Base image
FROM mcr.microsoft.com/windows/nanoserver:ltsc2022@sha256:1abc7e4d96a7dd1f80a31597abf6b98f112f1e24c2b448747e28bbea5ba97975
# Get administrator privileges
USER containeradministrator

# Create a fake symlink - This will act likea BUGCHECK if our layer import code tries to
# follow such symlinks.
RUN mklink /d C:\Users\Public\fakelink C:\abcd
# Create a directory and a file inside that directory. This file will be deleted in the next
# layer. The tests verify that even after file deletion in a layer timestamps are updated
# correcty.
RUN mkdir C:\Users\Public\testdir && echo "Windows Layer timestamp test" > C:\Users\Public\testdir\test.txt
RUN del /q C:\Users\Public\testdir\test.txt
