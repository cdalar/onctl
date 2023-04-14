#!/bin/bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o onctl-linux -ldflags="-w -s -X 'onkube/onctl/cmd.Version=$(git rev-parse HEAD | cut -c1-7)'"
cd ~/on/homebrew-tap
gh release delete-asset test onctl-linux -y
gh release upload test ~/on/onctl/onctl-linux
cd ~/on/onctl
