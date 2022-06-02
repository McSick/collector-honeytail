#!/bin/sh

until cd /logs && ls /logs/collector-logs.json
do
    echo "Retrying read from log file"
done

echo "Can now read from log file"
honeytail --writekey=$HONEYCOMB_WRITE_KEY --dataset=$HONEYCOMB_LOGS_DATASET --parser=json --file=/logs/collector-logs.json