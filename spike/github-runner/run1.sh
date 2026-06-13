#!/bin/bash
export GH_REPO=cdalar/onctl-runner-test
JIT_CONFIG=$(gh api -X POST "repos/${GH_REPO}/actions/runners/generate-jitconfig" \
	-f name=runner-spike-jit -F runner_group_id=1 \
	-f 'labels[]=self-hosted' -f 'labels[]=onctl' \
	-q .encoded_jit_config)
echo GH_REPO=$GH_REPO
echo JIT_CONFIG=$JIT_CONFIG

time onctl create -n runner-spike-jit \
	-a github-runner-jit.sh \
	-e JIT_CONFIG=$JIT_CONFIG
