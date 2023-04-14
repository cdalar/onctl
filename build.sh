#!/bin/bash
go build -ldflags="-w -s -X 'cdalar/onctl/cmd.Version=$(git rev-parse HEAD | cut -c1-7)'"
