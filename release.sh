#!/usr/bin/env bash
version=$(git rev-list -n 1 $(git describe  --abbrev=0 --tags) | cut -c1-7)
snapcraft list-revisions fluent-amqp | grep $version  | awk '{print $1}' | xargs -n 1 -i snapcraft release fluent-amqp '{}' beta,candidate,stable
