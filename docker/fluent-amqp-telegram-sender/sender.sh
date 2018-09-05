#!/bin/sh
set -e -o pipefail
while true; do
  cat template.txt | amqp-recv -o template "${BROKER_ROUTING_KEY}" > message.txt
  curl -f -X POST --data "text=$(cat message.txt)" --data "chat_id=${CHAT_ID}" "https://api.telegram.org/bot${TOKEN}/sendMessage" || exit 1
done