# CI

Herald's CI pipeline runs on every push and pull request targeting `main`. Four jobs run independently and in parallel.

## Jobs

| Job | Tool | Fails when |
|-----|------|------------|
| `lint` | `go vet` + staticcheck | Code quality issues found |
| `vulncheck` | govulncheck | Reachable vulnerability in dependencies |
| `build` | `go build` | Compilation fails (`CGO_ENABLED=0`) |
| `test` | `go test -race` | Tests fail or race condition detected |

All jobs share the same triggers:

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
```

For the full CI/CD overview including the release workflow, see the CI/CD section in [AGENTS.md](../AGENTS.md).

## Vulnerability Scanning

The `vulncheck` job uses [golang/govulncheck-action@v1](https://github.com/golang/govulncheck-action) to scan dependencies against the [Go vulnerability database](https://vuln.go.dev/). Unlike simple dependency scanners, govulncheck performs reachable code analysis -- it only flags vulnerabilities in functions the project actually calls. For Herald's small dependency set (go-telegram/bot, cobra, goldmark, bbolt), scans complete quickly.

No secrets, tokens, or configuration are needed. The action reads the Go version from `go.mod` and handles installation internally.

### Job definition

```yaml
vulncheck:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: golang/govulncheck-action@v1
```

### Action inputs

All inputs are optional. Herald uses the defaults.

| Input | Default | Effect |
|-------|---------|--------|
| `go-version-input` | reads `go.mod` | Go version to install for analysis |
| `go-package` | `./...` | Package pattern to scan |
| `check-latest` | `false` | Whether to check for latest Go patch version |
| `repo-checkout` | `true` | Whether the action checks out the repo |

## Test Coverage

The `test` job runs with the race detector enabled (`CGO_ENABLED=1`) and produces a coverage profile. After tests pass:

1. `go tool cover -func` parses `coverage.out` and writes the total percentage to the GitHub Actions step summary.
2. On `main` only, [schneegans/dynamic-badges-action@v1.7.0](https://github.com/Schneegans/dynamic-badges-action) writes the coverage percentage to a GitHub Gist. The shields.io badge in README reads from that Gist.

| Name | Kind | Purpose |
|------|------|---------|
| `GIST_TOKEN` | Repository secret | PAT with `gist` scope for badge updates |
| `COVERAGE_GIST_ID` | Repository variable | ID of the Gist storing `coverage.json` |

## Local Equivalents

Run the same checks locally before pushing.

```bash
# Lint
go vet ./...

# Vulnerability scan (one-time install)
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# Build
CGO_ENABLED=0 go build ./cmd/herald

# Test with race detector
go test -race ./...
```

To scan a specific package:

```bash
govulncheck ./internal/provider/...
```

## Interpreting Vulnerability Findings

When the vulncheck job fails, the log shows output like:

```
Vulnerability #1: GO-2024-XXXX
    Description of the vulnerability
    More info: https://pkg.go.dev/vuln/GO-2024-XXXX
    Module: github.com/example/dep
    Found in: github.com/example/dep@v1.2.3
    Fixed in: github.com/example/dep@v1.2.4
    Example trace found in:
      - herald/internal/provider.SomeFunction
```

Key fields:
- **Found in** -- the vulnerable version currently in `go.mod`
- **Fixed in** -- the minimum version that resolves the issue
- **Example trace** -- the call path proving the vulnerability is reachable

### Resolving a vulnerability

1. Check the advisory link for context and severity.
2. Update the dependency:
   ```bash
   go get github.com/example/dep@v1.2.4
   go mod tidy
   ```
3. Confirm the fix locally:
   ```bash
   govulncheck ./...
   ```
4. Commit the updated `go.mod` and `go.sum`.

If no fixed version exists yet, evaluate whether the vulnerable code path can be avoided or whether the dependency should be replaced.

## Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| vulncheck fails but you did not change any dependencies | New CVE published for an existing dependency | Update the affected dependency as described above |
| Govulncheck reports a suspected false positive | Rare with reachable code analysis | Run `govulncheck -show verbose ./...` locally; open a GitHub Issue if confirmed |
| vulncheck fails with an action or setup error | Infrastructure issue (network, runner) | Re-run the job from the GitHub Actions UI; check the [action repo](https://github.com/golang/govulncheck-action) for known issues |
| Coverage badge not updating | Gist write failed | Verify `GIST_TOKEN` secret is valid and has `gist` scope; check `COVERAGE_GIST_ID` variable matches the target Gist |
