# Fluent AMQP
[![](https://godoc.org/github.com/reddec/fluent-amqp?status.svg)](https://godoc.org/github.com/reddec/fluent-amqp)

Library that provides fluent and easy wrapper over [streadway-amqp](https://github.com/streadway/amqp) API.
Adds such features like:

- Reconnectiong. Will restore all defined infrastructure
- Non-blocking processing of messages
- Optional auto-requeue (with delay)
- Signing and verifiying messages by public/private pair

[API documentation](https://godoc.org/github.com/reddec/fluent-amqp)

## Signing and verification

Hash algorithm (x509) SHA512 with RSA, sign - SHA512.

The signer (producer) should use private key to sign content of message body and message id.

The validator (consumer) should use public certificate to validate content of message and message id against signature and should drops invalid or duplicated messages.

The sign should be put to a header (`X-Signature` by default: see `DefaultSignatureHeader` constant in godoc) as __binary__ object (**not hex or base64 encoded**).

```
DATA = BYTES(ID) ... BYTES(BODY)
# SIGN via PKCS#1 v1.5
SIGN_HEADER_BODY = SIGN_SHA512(PRIVATE_KEY, DATA)
```

