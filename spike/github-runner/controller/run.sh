#!/bin/bash
export GH_REPO=cdalar/onctl-runner-test
: "${WEBHOOK_SECRET:?set WEBHOOK_SECRET to the value used in webhook.sh}"

go run .
