# Test image for graceful termination CRI integration tests.
# `docker build -t cplatpublic.azurecr.io/servercore-gracefultermination-repro:latest .`
# `docker push cplatpublic.azurecr.io/servercore-gracefultermination-repro:latest`

FROM  golang:latest as build
ENV GOOS=windows
ENV GOARCH=amd64
ENV GO111MODULE=off
WORKDIR /app
COPY ./delayed-shutdown.go ./
RUN go build -o delayed-shutdown.exe

FROM mcr.microsoft.com/windows/servercore:ltsc2022@sha256:301d571fc0cffed67083d4cc7a10167f551212f76b3f91eb7457e89aa5a183da
WORKDIR /app
COPY --from=build /app/delayed-shutdown.exe .
ENTRYPOINT ["delayed-shutdown.exe"]
