package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/agent"
	"github.com/FengYuchen1314/sd-wan-plus/internal/ports"
)

func main() {
	nodePort := ports.Node
	if v := os.Getenv("PW_NODE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			nodePort = n
		}
	}
	cfg := agent.Config{
		NodeID:      os.Getenv("PW_NODE_ID"),
		ParentURL:   os.Getenv("PW_PARENT_URL"),
		DataDir:     os.Getenv("PW_DATA_DIR"),
		ArtifactDir: os.Getenv("PW_ARTIFACT_DIR"),
		NetdAddr:    os.Getenv("PW_NETD_ADDR"),
		NodePort:    nodePort,
		Root:        os.Getenv("PW_ROOT"),
		Interval:    15 * time.Second,
		ServeChildren: os.Getenv("PW_SERVE_CHILDREN") != "0",
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/opt/pathweaver/data"
	}
	if cfg.Root == "" {
		cfg.Root = "/opt/pathweaver"
	}
	a := agent.New(cfg)
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
