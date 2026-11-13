package wolfi

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"

	"github.com/quenchworks/images/tools/qw/internal/core"
)

// cmdWolfiLatest keeps the wolfi-latest.py command line: <pkg> [N] | --majors <prefix> [N] | --match <re>.
func Run(args []string) int {
	idx, err := core.LoadWolfi()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	switch {
	case len(args) > 1 && args[0] == "--majors":
		n := 4
		if len(args) > 2 {
			n, _ = strconv.Atoi(args[2])
		}
		ms, _ := core.WolfiMajors(args[1], n)
		for _, m := range ms {
			fmt.Fprintf(w, "%s  ->  %s\n", m[0], m[1])
		}
	case len(args) > 1 && args[0] == "--match":
		re := regexp.MustCompile(args[1])
		var names []string
		for p := range idx {
			if re.MatchString(p) {
				names = append(names, p)
			}
		}
		sort.Strings(names)
		for _, p := range names {
			fmt.Fprintf(w, "%s\t%s\n", p, core.BestPerRelease(idx[p])[0])
		}
	case len(args) > 0:
		n := 3
		if len(args) > 1 {
			n, _ = strconv.Atoi(args[1])
		}
		vs, ok := idx[args[0]]
		if !ok {
			fmt.Fprintf(os.Stderr, "NOT IN WOLFI: %s\n", args[0])
			return 1
		}
		for i, v := range core.BestPerRelease(vs) {
			if i == n {
				break
			}
			fmt.Fprintln(w, v)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: qw wolfi-latest <pkg> [N] | --majors <prefix> [N] | --match <regex>")
		return 2
	}
	return 0
}
