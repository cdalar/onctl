# Contributing to onctl

Thanks for your interest in onctl. This page describes how to report
problems and what a contribution needs in order to be accepted.

## Reporting bugs and requesting features

- Open a [GitHub issue](https://github.com/cdalar/onctl/issues) for bugs and
  feature requests. Include the onctl version (`onctl version`), the cloud
  provider, and the steps to reproduce.
- Do not report security vulnerabilities in public issues. Follow
  [SECURITY.md](SECURITY.md) instead.

## Submitting changes

1. Fork the repository and create a branch off `main`.
2. Make your change and add tests for it (see below).
3. Run the checks locally:

   ```bash
   make          # build
   make test     # go test ./...
   make lint     # golangci-lint run
   ```

4. Open a pull request against `main` and describe what changed and why.

Pull requests are merged once the required CI checks (build, tests, lint,
`govulncheck`) pass.

## Requirements for acceptable contributions

- **Coding standard:** code must be formatted with `gofmt` and pass
  `go vet` and `golangci-lint` without warnings. Follow the conventions in
  [AGENTS.md](AGENTS.md).
- **Tests:** new functionality and bug fixes must come with automated tests
  in the same pull request. Tests use the standard `go test` framework with
  `testify`. Code that can only be exercised against a real cloud account
  should keep its logic behind interfaces so the logic itself stays testable.
- **Dependencies:** `go.mod` and `go.sum` must be tidy (`go mod tidy`).
- **License:** contributions are accepted under the project's
  [AGPL-3.0 license](LICENSE).
- **Conduct:** participation is governed by the
  [Code of Conduct](CODE_OF_CONDUCT.md).
