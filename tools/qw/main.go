// qw is the QuenchWorks images tooling: upstream version checks and the Wolfi index.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: qw wolfi-latest ... | qw check [app...] | qw vex [--stale] [app...] | qw gen-catalog [--meta] [-o file]")
		os.Exit(2)
	}
	if wd, err := os.Getwd(); err == nil {
		imagesRoot = findRoot(wd)
	}
	switch os.Args[1] {
	case "wolfi-latest":
		os.Exit(cmdWolfiLatest(os.Args[2:]))
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "gen-catalog":
		os.Exit(cmdGenCatalog(os.Args[2:]))
	case "vex":
		os.Exit(cmdVex(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "qw: unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

// findRoot walks up to the directory holding apps/ and catalog.yaml.
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
