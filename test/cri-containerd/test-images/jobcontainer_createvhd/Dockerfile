# Irrelevant what image version we use for job containers as there's no container <-> host OS version restraint.
FROM golang:1.15.10-nanoserver-1809

# Get administrator privileges
USER containeradministrator

WORKDIR C:\\go\\src\\vhd
COPY main.go .

RUN go get -d -v ./...
RUN go build -mod=mod