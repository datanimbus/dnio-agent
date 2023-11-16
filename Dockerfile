###############################################################################################
# Go Agent Build
###############################################################################################

FROM golang:1.18-alpine AS agents

ARG SIGNING_KEY_USER=dev
ARG SIGNING_KEY_PASSWORD=dev

ENV GOPROXY=direct

RUN apk add git
# RUN apk add make

WORKDIR /app

COPY . .

# Building Executables
# Mac Build
RUN env GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o exec/datastack-agent-darwin-amd64 . || true
# Linux Build
RUN env GOOS=linux GOARCH=386 go build -ldflags="-s -w" -o exec/datastack-agent-linux-386 .
RUN env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o exec/datastack-agent-linux-amd64 . || true
# Windows Build
RUN env GOOS=windows GOARCH=386 go build -ldflags="-s -w" -o exec/datastack-agent-windows-386-unsigned.exe .
RUN env GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o exec/datastack-agent-windows-amd64-unsigned.exe .

###############################################################################################
#Agent Signing
###############################################################################################

FROM ubuntu:20.04 AS oss

RUN apt-get update
RUN apt-get install -y osslsigncode
RUN apt-get install -y wget

WORKDIR /app

RUN wget --user ${SIGNING_KEY_USER} --password ${SIGNING_KEY_PASSWORD} https://dev.datanimbus.io/agentbuild/out.key
RUN wget --user ${SIGNING_KEY_USER} --password ${SIGNING_KEY_PASSWORD} https://dev.datanimbus.io/agentbuild/cd786349a667ff05-SHA2.pem

COPY --from=agents /app/exec ./exec
COPY --from=agents /app/scriptFiles ./scriptFiles

RUN osslsigncode -h sha2 -certs cd786349a667ff05-SHA2.pem -key out.key -t http://timestamp.comodoca.com/authenticode -in exec/datastack-agent-windows-386-unsigned.exe -out exec/datastack-agent-windows-386.exe
RUN osslsigncode -h sha2 -certs cd786349a667ff05-SHA2.pem -key out.key -t http://timestamp.comodoca.com/authenticode -in exec/datastack-agent-windows-amd64-unsigned.exe -out exec/datastack-agent-windows-amd64.exe