#!/bin/bash
go build -ldflags="-w -s -X 'onkube/onctl/cmd.Version=$(git rev-parse HEAD | cut -c1-7)'"
