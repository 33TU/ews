// Command goversion prints runtime.Version(), which names the Go toolchain
// together with any GOEXPERIMENT it was built with, such as go1.27.1-X:simd,
// where `go version` prints the release alone. The benchmark recipes record
// it at the top of the raw output.
package main

import (
	"fmt"
	"runtime"
)

func main() { fmt.Println(runtime.Version()) }
