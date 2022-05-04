#!/bin/bash
if [ -z "$1" ]
then
    echo -n "Password:" 
    read -s password
fi
if [ ! -z "$1" ] && [ $1 = "-p" ]
then
    password=$2
fi
./bin/datastack-sentinel -p $password -service stop
./bin/datastack-sentinel -p $password -service uninstall
./bin/datastack-sentinel -p $password -service install
./bin/datastack-sentinel -p $password -service start
./bin/datastack-agent -p $password -c ./conf/agent.conf -service stop
./bin/datastack-agent -p $password -c ./conf/agent.conf -service uninstall
./bin/datastack-agent -p $password -c ./conf/agent.conf -service install
./bin/datastack-agent -p $password -c ./conf/agent.conf -service start