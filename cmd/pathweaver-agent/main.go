package main

import (
	"log"
	"os"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/agent"
)

func main() {
	cfg := agent.Config{
		NodeID:    os.Getenv("PW_NODE_ID"),
		ParentURL: os.Getenv("PW_PARENT_URL"),
		DataDir:   os.Getenv("PW_DATA_DIR"),
		NetdAddr:  os.Getenv("PW_NETD_ADDR"),
		Interval:  15 * time.Second,
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/opt/pathweaver/data"
	}
	a := agent.New(cfg)
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
