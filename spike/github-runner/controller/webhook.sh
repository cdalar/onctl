#!/bin/bash
set -x
export GH_REPO=cdalar/onctl-runner-test
: "${SMEE_CHANNEL:?get one from https://smee.io/new and export SMEE_CHANNEL=<channel>}"
export WEBHOOK_SECRET=$(openssl rand -hex 20)
echo "WEBHOOK_SECRET=$WEBHOOK_SECRET"   # export this for run.sh too

gh api repos/$GH_REPO/hooks -X POST \
  -f name=web \
  -f "config[url]=https://smee.io/$SMEE_CHANNEL" \
  -f 'config[content_type]=json' \
  -f "config[secret]=$WEBHOOK_SECRET" \
  -f 'events[]=workflow_job'
