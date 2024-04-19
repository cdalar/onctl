#!/bin/bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o onctl-linux -ldflags="-w -s -X 'github.com/cdalar/onctl/cmd.Version=$(git rev-parse HEAD | cut -c1-7)'"
GOOS=windows GOARCH=amd64 go build -o onctl.exe -ldflags="-w -s -X 'github.com/cdalar/onctl/cmd.Version=$(git rev-parse HEAD | cut -c1-7)'"
# GOOS=darwin GOARCH=amd64 go build -o onctl-mac -ldflags="-w -s -X 'github.com/cdalar/onctl/cmd.Version=$(git rev-parse HEAD | cut -c1-7)'"
gh release delete-asset v0.1.0 onctl.exe -y
gh release upload v0.1.0 onctl.exe
gh release delete-asset v0.1.0 onctl-linux -y
gh release upload v0.1.0 onctl-linux

