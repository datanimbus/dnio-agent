
###############################################################################################
# Go Agent Build
###############################################################################################

FROM golang:1.14 AS agents

# RUN apk add git
# RUN apk add make

WORKDIR /app

RUN go get -u github.com/gorilla/mux
RUN go get -u github.com/asdine/storm
RUN go get -u github.com/satori/go.uuid
RUN go get -u github.com/howeyc/fsnotify
RUN go get -u github.com/howeyc/gopass
RUN go get -u go.mongodb.org/mongo-driver/bson
RUN go get -u go.mongodb.org/mongo-driver/mongo
RUN go get -u github.com/appveen/govault
RUN go get -u github.com/appveen/go-log/log
RUN go get -u github.com/kardianos/service
RUN go get -u github.com/nats-io/go-nats-streaming
RUN go get -u github.com/nats-io/go-nats
RUN go get -u github.com/robfig/cron
RUN go get -u github.com/iancoleman/strcase
RUN go get -u golang.org/x/time/rate

COPY . .

# Building Executables
# Mac Build
RUN env GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o exec/datastack-agent-darwin-amd64 ./v1 || true
# Linux Build
RUN env GOOS=linux GOARCH=386 go build -ldflags="-s -w" -o exec/datastack-agent-linux-386 ./v1
RUN env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o exec/datastack-agent-linux-amd64 ./v1 || true
# Windows Build
RUN env GOOS=windows GOARCH=386 go build -ldflags="-s -w" -o exec/datastack-agent-windows-386-unsigned.exe ./v1
RUN env GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o exec/datastack-agent-windows-amd64-unsigned.exe ./v1

###############################################################################################
#Agent Signing
###############################################################################################

FROM ubuntu AS oss

RUN apt-get update
RUN apt-get install -y osslsigncode

WORKDIR /app

COPY --from=agents /app/exec ./exec
COPY --from=agents /app/certs ./certs
COPY --from=agents /app/scriptFiles ./scriptFiles

RUN osslsigncode -h sha2 -certs certs/cd786349a667ff05-SHA2.pem -key certs/out.key -t http://timestamp.comodoca.com/authenticode -in exec/datastack-agent-windows-386-unsigned.exe -out exec/datastack-agent-windows-386.exe
RUN osslsigncode -h sha2 -certs certs/cd786349a667ff05-SHA2.pem -key certs/out.key -t http://timestamp.comodoca.com/authenticode -in exec/datastack-agent-windows-amd64-unsigned.exe -out exec/datastack-agent-windows-amd64.exe