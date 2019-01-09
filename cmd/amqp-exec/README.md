# amqp-exec

It's like CGI but for AMQP protocol:

1. Listening for message from broker as `amqp-recv`
2. Executes application initially defined by arguments. The app STDIN mapped to message body.
3. Wait for finish
4. If exit code is not 0 then requeue back to AMQP broker with some delay
5. Otherwise if REPLY_TO property defined sends full STDOUT to corresponded 
REPLY_TO routing key and correlation id

Note: you may pass `--` before positional arguments to prevent interference with arguments of `amqp-exec` itself.

Example:


* `amqp-exec -Q qwe -- date --rfc-3339=seconds`
generates current date time in RFC339 with seconds precision
