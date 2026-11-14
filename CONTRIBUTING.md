# Contributing to Skein

Issues and pull requests are welcome. Skein is an execution control plane for local runs, not an agent framework: keep changes focused on scheduling, the epoch fence and the stop guarantee.

## Checks

CI runs on every push and pull request against `main`:

```bash
go test -count=1 ./...
go vet ./...
gofmt -l .        # must print nothing
```

The race detector needs CGO and a C compiler:

```bash
CGO_ENABLED=1 go test -race -count=1 ./...
```

## Build

Go 1.26 or newer is required, as pinned in `go.mod`.

```bash
go build -trimpath -o bin/skein ./cmd/skein
go build -trimpath ./cmd/skein ./cmd/skein-bench
```

The landing page under `site/` is a separate Node build:

```bash
cd site
npm ci
npm run build
```

## Runtime proofs

CI builds the binary and runs the functional and parent/child stop proofs on six native runners (Linux, Windows and macOS, amd64 and arm64):

```bash
go build -trimpath -o bin/skein ./cmd/skein
python bench/functional_proof.py --skein bin/skein --output bench/results/ci-functional-local.json
python bench/stop_proof.py --skein bin/skein --runs 3 --warmups 1 --tasks 4 --output bench/results/ci-stop-local.json
```

`bench/README.md` holds the full reproduction recipe. Keep the raw samples and the described environment with every measurement change, and never publish a number that was not measured.

## Propose a change

- Open an issue first when the guarantee or the contract is in question; open a pull request against `main` when it is not.
- Keep a pull request focused on one change and keep unrelated formatting out of it. `gofmt` must report no diff.
- Add or update tests for every behavioural change. Changes to the stop path or the epoch fence need a test that fails without the change.
- Do not add vendor links or endorsements to the documentation.
- Do not commit secrets, API keys or credentials. Use environment variables and local fixtures instead.

## License

MIT, see `LICENSE`. There is no separate contributor license agreement and no requirement to assign copyright.

## Security

Skein is not a sandbox: manifest commands execute with the permissions of the user who started the run. Report vulnerabilities privately through a GitHub security advisory for `Balanov/skein` (Security -> Report a vulnerability) rather than in a public issue, and include the affected version or commit, the operating system, reproduction steps and the guarantee you believe is crossed. See `SECURITY.md`.
