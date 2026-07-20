package main

import (
	"fmt"
	"os"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("pathweaver-cli %s\nusage: pathweaver-cli version\n", core.ProductVersion)
		os.Exit(0)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(core.ProductVersion)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
