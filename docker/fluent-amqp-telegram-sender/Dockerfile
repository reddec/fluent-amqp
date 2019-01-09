FROM reddec/fluent-amqp:v0.1.15
ENV TOKEN ''
ENV CHAT_ID ''
ENV BROKER_URL 'amqp://guest:guest@broker'
ENV BROKER_EXCHANGE 'notifications'
ENV BROKER_KIND 'topic'
ENV BROKER_QUEUE "telegram-sender"
ENV BROKER_ROUTING_KEY '#'
ENV SOCKS5 ""

RUN apk add --no-cache curl ca-certificates
ADD template.txt /
ADD sender.sh /
CMD ["/bin/sh", "/sender.sh"]