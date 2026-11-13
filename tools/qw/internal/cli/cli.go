// Package cli maps qw's command names to the packages that run them.
package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/quenchworks/images/tools/qw/internal/blocked"
	"github.com/quenchworks/images/tools/qw/internal/bump"
	"github.com/quenchworks/images/tools/qw/internal/catalog"
	"github.com/quenchworks/images/tools/qw/internal/check"
	"github.com/quenchworks/images/tools/qw/internal/core"
	"github.com/quenchworks/images/tools/qw/internal/drift"
	"github.com/quenchworks/images/tools/qw/internal/unbuilt"
	"github.com/quenchworks/images/tools/qw/internal/vex"
	"github.com/quenchworks/images/tools/qw/internal/wolfi"
)

var commands = map[string]struct {
	run         func([]string) int
	args, about string
}{
	"check":        {check.Run, "[-q] [-j N] [app...]", "upstream versions vs build.conf; exit code = updates"},
	"bump":         {bump.Run, "<app> <version>...", "add versions to a recipe with fresh shas"},
	"gen-catalog":  {catalog.Run, "[--meta] [-o file]", "write catalog.lock.yaml from GHCR"},
	"drift-scan":   {drift.Run, "[--shard N --of M] [--json f] [--only app...]", "rescan published images"},
	"vex":          {vex.Run, "[--stale] [app...]", "lint VEX files and their wiring"},
	"blocked":      {blocked.Run, "[--pkg P --min V] [--dispatch]", "re-test BLOCKED apps against Wolfi"},
	"unbuilt":      {unbuilt.Run, "[--all] [app...]", "VERSIONS entries missing on GHCR"},
	"wolfi-latest": {wolfi.Run, "<pkg> [N] | --majors P [N] | --match RE", "newest Wolfi versions"},
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: qw <command> [args]")
	for _, name := range slices.Sorted(maps.Keys(commands)) {
		c := commands[name]
		fmt.Fprintf(os.Stderr, "  %-13s %s\n  %-13s   %s\n", name, c.args, "", c.about)
	}
}

// Run executes one command line (without the program name) and returns its exit code.
func Run(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return 2
	}
	c, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "qw: unknown command %q\n", args[0])
		usage()
		return 2
	}
	if wd, err := os.Getwd(); err == nil {
		core.ImagesRoot = findRoot(wd)
	}
	return c.run(args[1:])
}

// findRoot walks up to the directory holding catalog.yaml.
func findRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "catalog.yaml")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}
