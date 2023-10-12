#!/usr/bin/env bash

if [[ -z "${UUID}" ]]; then
    echo "UUID is not set"
    echo "Usage: UUID=... ./client_bash.sh"
    exit 1
fi

curl -X POST -H "Content-Type: application/json" -d '{"msg":"Test msg from curl to /json", "encrypted":"false"}' "http://localhost:7888/api/${UUID}/json"

curl -X GET "http://localhost:7888/api/${UUID}/get?msg=Test&encrypted=false"

curl -X POST -H "Content-Type: x-www-form-urlencoded" -d 'msg=Test msg from curl to /post&encrypted=false' "http://localhost:7888/api/${UUID}/form"