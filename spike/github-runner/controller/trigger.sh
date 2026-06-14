#!/bin/bash
GH_REPO=cdalar/onctl-runner-test
gh workflow run onctl-test.yml -R $GH_REPO
#gh run watch -R $GH_REPO
