package main

import (
	"fmt"
	"os"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("pathweaver-cli %s\nusage: pathweaver-cli version|wg-keypair\n", core.ProductVersion)
		os.Exit(0)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(core.ProductVersion)
	case "wg-keypair":
		priv, pub, err := security.GenerateWGKeyPair()
		if err != nil {
			fmt.Fprintf(os.Stderr, "wg-keypair: %v\n", err)
			os.Exit(1)
		}
		// line1=private line2=public (base64, WireGuard format)
		fmt.Println(priv)
		fmt.Println(pub)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
