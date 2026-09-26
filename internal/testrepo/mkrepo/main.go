// Command mkrepo creates the fixture repository from package testrepo, to try
// buti against by hand:
//
//	go run ./internal/testrepo/mkrepo /tmp/buti-fixture
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bartinthefield/buti/internal/testrepo"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mkrepo <dir>")
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	r, err := testrepo.Create(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkrepo:", err)
		os.Exit(1)
	}
	fmt.Println("# fixture ready; run buti against it with:")
	fmt.Printf("env HOME=%s XDG_CONFIG_HOME=%s/.config XDG_DATA_HOME=%s/.local/share XDG_CACHE_HOME=%s/.cache go run ./cmd/buti -C %s\n",
		r.Home, r.Home, r.Home, r.Home, r.Dir)
}
