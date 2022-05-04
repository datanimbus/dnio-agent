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
./bin/datastack-agent -p $password -c ./conf/agent.conf