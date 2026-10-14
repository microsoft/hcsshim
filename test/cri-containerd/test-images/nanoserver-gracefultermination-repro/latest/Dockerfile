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

FROM mcr.microsoft.com/windows/nanoserver:ltsc2022@sha256:32729e1cdc6718c90952375e9bbae2b2c5d38f41ceb50a693fc2383b76df28fd
WORKDIR /app
COPY --from=build /app/delayed-shutdown.exe .
ENTRYPOINT ["delayed-shutdown.exe"]
