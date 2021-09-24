# This dockerfile builds a super barebones container image that includes a binary to do a single HNS operation to
# validate that we can actually talk to HNS in a job container. As this is a huge usecase for job containers this is paramount
# to test. The binary in the image will NOT function if this image is used for a normal windows container, both process and hypervisor isolated.

# Irrelevant what image version we use for job containers as there's no container <-> host OS version restraint.
FROM golang:1.15.10-nanoserver-1809

# Get administrator privileges
USER containeradministrator

WORKDIR C:\\go\\src\\workdir
COPY main.go .

RUN go get -d -v ./...
RUN go build -mod=mod