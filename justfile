# Development tasks for ews. Run `just` to list them.

set shell := ["bash", "-euo", "pipefail", "-c"]

scratch := "/tmp/ews-autobahn"

default:
    @just --list

# Format, vet, and test every package with the race detector.
check: fmt vet test

fmt:
    test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }

vet:
    go vet ./...
    cd bench && go vet ./...

test:
    go test -race ./...

# The same under GOEXPERIMENT=simd, which enables the SIMD mask and UTF-8 paths.
test-simd:
    GOEXPERIMENT=simd go test ./...

# Cross-compile checks for the targets with their own build tags.
cross:
    GOEXPERIMENT=simd GOARCH=arm64 go vet ./...
    GOEXPERIMENT=simd GOARCH=wasm GOOS=wasip1 go vet ./...
    GOARCH=386 go vet ./...

# Package micro-benchmarks with allocation counts.
bench pkg="./...":
    go test {{pkg}} -run '^$' -bench . -benchmem

bench-simd pkg="./...":
    GOEXPERIMENT=simd go test {{pkg}} -run '^$' -bench . -benchmem

# End-to-end echo comparison against other libraries; regenerates bench/echo/RESULTS.md.
bench-echo benchtime="500ms":
    cd bench/echo && go test -run '^$' -bench Echo -benchtime {{benchtime}} -timeout 3600s | go run ../cmd/results -benchtime {{benchtime}} > RESULTS.md

# The echo comparison built with GOEXPERIMENT=simd, written to bench/echo/RESULTS-simd.md.
bench-echo-simd benchtime="500ms":
    cd bench/echo && GOEXPERIMENT=simd go test -run '^$' -bench Echo -benchtime {{benchtime}} -timeout 3600s | go run ../cmd/results -benchtime {{benchtime}} > RESULTS-simd.md

# Broadcast comparison, one message to many connections; regenerates bench/broadcast/RESULTS.md.
bench-broadcast benchtime="1s":
    cd bench/broadcast && go test -run '^$' -bench Broadcast -benchtime {{benchtime}} -timeout 3600s | go run ../cmd/results -benchtime {{benchtime}} > RESULTS.md

# Fan-out benchmark: bursts of small messages per event, batched and unbatched.
bench-fanout:
    cd bench/broadcast && go test -run '^$' -bench Fanout -benchmem

# Run the Autobahn test suite against examples/echo (needs docker or podman).
autobahn:
    mkdir -p {{scratch}}/reports
    cp autobahn/fuzzingclient.json {{scratch}}/
    go build -o {{scratch}}/echo ./examples/echo
    {{scratch}}/echo -addr 127.0.0.1:9001 & echo $! > {{scratch}}/echo.pid
    sleep 0.5
    docker run --rm --network host -v {{scratch}}:/config:Z -v {{scratch}}/reports:/reports:Z \
        docker.io/crossbario/autobahn-testsuite wstest -m fuzzingclient -s /config/fuzzingclient.json \
        || true
    kill "$(cat {{scratch}}/echo.pid)"
    python3 -c 'import json,collections; r=json.load(open("{{scratch}}/reports/index.json"))["ews"]; print(len(r), "cases:", dict(collections.Counter(v["behavior"] for v in r.values())))'

tidy:
    go mod tidy
    cd bench && go mod tidy
