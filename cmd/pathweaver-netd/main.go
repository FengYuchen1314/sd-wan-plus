package main

import (
	"log"
	"os"

	"github.com/FengYuchen1314/sd-wan-plus/internal/netd"
)

func main() {
	sock := os.Getenv("PW_NETD_SOCKET")
	s := netd.New(sock)
	log.Fatal(s.ListenAndServe())
}
