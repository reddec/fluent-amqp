FROM reddec/fluent-amqp:v0.1.1
ENV TOKEN ''
ENV CHAT_ID ''
ENV BROKER_URL 'amqp://guest:guest@broker'
ENV BROKER_EXCHANGE 'notifications'
ENV BROKER_KIND 'topic'
ENV BROKER_ROUTING_KEY '#'

RUN apk add --no-cache curl ca-certificates
ADD template.txt /
ADD sender.sh /
CMD ["/bin/sh", "/sender.sh"]