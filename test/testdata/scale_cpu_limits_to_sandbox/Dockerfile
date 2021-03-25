# This Dockerfile builds a docker image based on golang:1.16.2-nanoserver-1809.
# The image is used in test/cri-containerd/scale_cpu_limits_to_sandbox.go.
# If any changes are made to this Dockerfile, make sure to update the tests
# accordingly.

# Base image
FROM golang:1.16.2-nanoserver-1809

# Get administrator privileges
USER ContainerAdministrator

# Put everything in the root directory
WORKDIR /

# Copy the source file
COPY main.go .

# Build binary
RUN go build -o load_cpu.exe main.go
