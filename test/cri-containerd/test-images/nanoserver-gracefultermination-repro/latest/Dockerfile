FROM  golang:latest as build
ENV GOOS=windows
ENV GOARCH=amd64
ENV GO111MODULE=off
WORKDIR /app
COPY ./delayed-shutdown.go ./
RUN go build -o delayed-shutdown.exe

FROM mcr.microsoft.com/windows/nanoserver:ltsc2022
WORKDIR /app
COPY --from=build /app/delayed-shutdown.exe .
ENTRYPOINT ["delayed-shutdown.exe"]
