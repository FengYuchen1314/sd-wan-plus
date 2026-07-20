package main

import (
	"log"
	"os"

	"github.com/FengYuchen1314/sd-wan-plus/internal/updater"
)

func main() {
	root := os.Getenv("PW_ROOT")
	u := updater.New(root)
	log.Fatal(u.RunLoop())
}
