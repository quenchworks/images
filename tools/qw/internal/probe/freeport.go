package probe

import (
	"fmt"
	"net"
	"os"
)

// FreePort prints a TCP port on 127.0.0.1 that is free right now (smoke tests bind
// containers to it). Another process can take it before the caller does; the tests retry.
func FreePort(args []string) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "freeport:", err)
		return 1
	}
	defer l.Close()
	fmt.Println(l.Addr().(*net.TCPAddr).Port)
	return 0
}
