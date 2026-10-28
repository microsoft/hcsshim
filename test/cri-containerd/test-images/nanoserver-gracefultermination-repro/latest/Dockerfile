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

FROM mcr.microsoft.com/windows/nanoserver:ltsc2022@sha256:62d28cb7771b0cf1b37db2662e468dad76e7ef15ca4ef52b2ab73c102a23da98
WORKDIR /app
COPY --from=build /app/delayed-shutdown.exe .
ENTRYPOINT ["delayed-shutdown.exe"]
