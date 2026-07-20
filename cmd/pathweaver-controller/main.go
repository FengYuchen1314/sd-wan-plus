package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/FengYuchen1314/sd-wan-plus/internal/controller"
	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
)

func main() {
	bootstrap := flag.Bool("bootstrap", false, "initialize controller database")
	password := flag.String("password", "", "admin password (bootstrap)")
	name := flag.String("name", "controller", "controller node name (bootstrap)")
	flag.Parse()

	cfg := controller.DefaultConfig()
	if v := os.Getenv("PW_WEB_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.WebPort = n
		}
	}
	if v := os.Getenv("PW_NODE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.NodePort = n
		}
	}
	if v := os.Getenv("PW_WG_PORT_START"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.WGPortStart = n
		}
	}
	if v := os.Getenv("PW_WG_PORT_END"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.WGPortEnd = n
		}
	}
	if v := os.Getenv("PW_PUBLIC_ADDRESS"); v != "" {
		cfg.PublicAddress = v
	}
	if v := os.Getenv("PW_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("PW_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("PW_KEY_FILE"); v != "" {
		cfg.KeyFile = v
	}
	if v := os.Getenv("PW_STATIC_DIR"); v != "" {
		cfg.StaticDir = v
	}
	if v := os.Getenv("PW_OVERLAY_CIDR"); v != "" {
		cfg.OverlayCIDR = v
	}
	if os.Getenv("PW_TLS") == "1" {
		cfg.TLS = true
	}

	srv, err := controller.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	if *bootstrap {
		if *password == "" {
			fmt.Fprintln(os.Stderr, "--password required with --bootstrap")
			os.Exit(1)
		}
		if err := controller.BootstrapController(srv.DB(), srv.Box(), cfg, *password, *name); err != nil {
			log.Fatal(err)
		}
		fmt.Println("bootstrap complete")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("PathWeaver controller %s", core.ProductVersion)
	if err := srv.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
	}
}
