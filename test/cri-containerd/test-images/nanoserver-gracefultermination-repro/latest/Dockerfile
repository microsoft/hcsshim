# Test image for graceful termination CRI integration tests.
# `docker build -t cplatpublic.azurecr.io/nanoserver-gracefultermination-repro:latest .`
# `docker push cplatpublic.azurecr.io/nanoserver-gracefultermination-repro:latest`

FROM  golang:latest as build
ENV GOOS=windows
ENV GOARCH=amd64
ENV GO111MODULE=off
WORKDIR /app
COPY ./delayed-shutdown.go ./
RUN go build -o delayed-shutdown.exe

FROM mcr.microsoft.com/windows/nanoserver:ltsc2022@sha256:5a1c3c8308f8f5c9b67d21d2332a449d4ab8f9a6cf277f07d4aeac27ff2a0213
WORKDIR /app
COPY --from=build /app/delayed-shutdown.exe .
ENTRYPOINT ["delayed-shutdown.exe"]
