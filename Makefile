GO_CMD=go
BINARY_NAME=onctl

# Mark targets as phony (not files)
.PHONY: all build clean run test

# Default target
all: build

# Build the binary
build:
	$(GO_CMD) mod tidy
	time $(GO_CMD) build -ldflags="-w -s -X 'github.com/cdalar/onctl/cmd.Version=`git rev-parse HEAD | cut -c1-7`' \
		-X 'github.com/cdalar/onctl/cmd.BuildTime=`date -u '+%Y-%m-%d %H:%M:%S'`' \
		-X 'github.com/cdalar/onctl/cmd.GoVersion=`go version`'" \
		-o $(BINARY_NAME) main.go

# Clean up the binary
clean:
	rm $(BINARY_NAME)

# Test the application
test:
	$(GO_CMD) test ./...
