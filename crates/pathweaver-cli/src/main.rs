use clap::Parser;

fn main() -> anyhow::Result<()> {
    let cli = pathweaver_cli::Cli::parse();

    match cli.command {
        pathweaver_cli::Commands::Controller { config } => {
            println!("Starting controller with config: {}", config);
        }
        pathweaver_cli::Commands::Agent { config } => {
            println!("Starting agent with config: {}", config);
        }
        pathweaver_cli::Commands::InitController {
            name,
            admin_password,
            web_port,
            node_service_port,
            wg_port_start,
            wg_port_end,
            public_address,
            overlay_ipv4_cidr,
            ipv6_enabled,
            tls_mode,
        } => {
            println!("Initializing controller:");
            println!("  Name: {}", name);
            println!("  Web port: {}", web_port);
            println!("  Node service port: {}", node_service_port);
            println!("  WG port range: {}-{}", wg_port_start, wg_port_end);
            println!("  Public address: {}", public_address);
            println!("  Overlay CIDR: {}", overlay_ipv4_cidr);
            println!("  IPv6 enabled: {}", ipv6_enabled);
            println!("  TLS mode: {}", tls_mode);

            let password_hash = pathweaver_security::auth::hash_password(&admin_password)?;
            println!("\n  Password hash created successfully");
        }
        pathweaver_cli::Commands::GenerateToken {
            parent_node_id,
            node_name,
            ttl_seconds,
        } => {
            let token = pathweaver_security::crypto::generate_random_token(32);
            println!("Generated enrollment token:");
            println!("  Parent: {}", parent_node_id);
            println!("  Node name: {}", node_name);
            println!("  TTL: {}s", ttl_seconds);
            println!("  Token: {}", token);
        }
        pathweaver_cli::Commands::CheckUpdate => {
            println!("Checking for updates...");
        }
    }

    Ok(())
}
