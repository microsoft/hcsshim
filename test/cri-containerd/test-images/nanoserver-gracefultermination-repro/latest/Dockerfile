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

FROM mcr.microsoft.com/windows/nanoserver:ltsc2022@sha256:e4ce8c20390c3785c3cbeef15c579d186b3599d37525c596590cf4508e38d3ff
WORKDIR /app
COPY --from=build /app/delayed-shutdown.exe .
ENTRYPOINT ["delayed-shutdown.exe"]
