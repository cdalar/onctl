FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s \
      -X 'github.com/cdalar/onctl/cmd.Version=$(git rev-parse --short HEAD 2>/dev/null || echo dev)' \
      -X 'github.com/cdalar/onctl/cmd.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)' \
      -X 'github.com/cdalar/onctl/cmd.GoVersion=$(go version)'" \
    -o onctl main.go

FROM alpine:3.21

RUN apk add --no-cache ca-certificates openssh-client

COPY --from=builder /app/onctl /usr/local/bin/onctl

ENTRYPOINT ["onctl"]
