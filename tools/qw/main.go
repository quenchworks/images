// qw is the QuenchWorks images tooling: upstream version checks, recipe bumps, the
// catalog lock, CVE drift and VEX checks, and the Wolfi index. Run it through the
// Makefile next to this file, or as `go -C tools/qw run . <command>`.
package main

import (
	"os"

	"github.com/quenchworks/images/tools/qw/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
