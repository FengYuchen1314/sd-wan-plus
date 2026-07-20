use clap::{Parser, Subcommand};

#[derive(Parser)]
#[command(name = "pathweaver")]
#[command(about = "PathWeaver SD-WAN control plane")]
pub struct Cli {
    #[command(subcommand)]
    pub command: Commands,
}

#[derive(Subcommand)]
pub enum Commands {
    Controller {
        #[arg(long, default_value = "config.toml")]
        config: String,
    },

    Agent {
        #[arg(long, default_value = "config.toml")]
        config: String,
    },

    InitController {
        #[arg(long)]
        name: String,

        #[arg(long)]
        admin_password: String,

        #[arg(long, default_value_t = 8443)]
        web_port: u16,

        #[arg(long, default_value_t = 8444)]
        node_service_port: u16,

        #[arg(long, default_value_t = 30000)]
        wg_port_start: u16,

        #[arg(long, default_value_t = 30999)]
        wg_port_end: u16,

        #[arg(long)]
        public_address: String,

        #[arg(long, default_value = "10.250.0.0/16")]
        overlay_ipv4_cidr: String,

        #[arg(long)]
        ipv6_enabled: bool,

        #[arg(long, default_value = "auto-generated")]
        tls_mode: String,
    },

    GenerateToken {
        #[arg(long)]
        parent_node_id: String,

        #[arg(long)]
        node_name: String,

        #[arg(long, default_value_t = 600)]
        ttl_seconds: u64,
    },

    CheckUpdate,
}
