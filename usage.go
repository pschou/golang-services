package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func init() {
	flag.Usage = func() {
		lines := strings.SplitN(about, "\n", 2)
		fmt.Fprintf(os.Stderr, "%s (github.com/pschou/goproxy-service, version: %s)\n%s\n\nUsage: %s %s\n",
			lines[0], compileVersion, lines[1], os.Args[0], usage)

		flag.PrintDefaults()
	}
}
