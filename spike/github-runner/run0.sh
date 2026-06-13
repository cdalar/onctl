#!/bin/bash
export GH_REPO=cdalar/onctl-runner-test
TOKEN=$(gh api -X POST "repos/${GH_REPO}/actions/runners/registration-token" -q .token)
echo GH_REPO=$GH_REPO
echo TOKEN=$TOKEN

time onctl create -n runner-spike \
	-a github-runner.sh \
	-e GH_REPO=$GH_REPO -e RUNNER_TOKEN=$TOKEN
