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

FROM mcr.microsoft.com/windows/servercore:ltsc2022@sha256:029d8eb737c978e3df0f26abb529efd38d3c9ecee9d8d0bba6d0c205f32dc263
WORKDIR /app
COPY --from=build /app/delayed-shutdown.exe .
ENTRYPOINT ["delayed-shutdown.exe"]
