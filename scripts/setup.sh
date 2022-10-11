#!/bin/sh

if [ $WORKSPACE ]; then
    cd $WORKSPACE
fi

echo "****************************************************"
echo "data.stack.b2b.agents :: Fetching dependencies"
echo "****************************************************"
go get -u github.com/gorilla/mux
go get -u github.com/asdine/storm
go get -u github.com/satori/go.uuid
go get -u github.com/howeyc/fsnotify
go get -u github.com/howeyc/gopass
go get -u go.mongodb.org/mongo-driver/
go get -u github.com/appveen/govault
go get -u github.com/appveen/go-log/log
go get -u github.com/kardianos/service
go get -u github.com/nats-io/go-nats-streaming
go get -u github.com/nats-io/go-nats
go get -u github.com/robfig/cron
go get -u github.com/iancoleman/strcase
go get -u golang.org/x/time/rate
echo "all dependencies fetched"
