#!/bin/bash

: "${SMEE_CHANNEL:?get one from https://smee.io/new and export SMEE_CHANNEL=<channel>}"
npx smee-client --url "https://smee.io/$SMEE_CHANNEL" --target http://localhost:8080/webhook
