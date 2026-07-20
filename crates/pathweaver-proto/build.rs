use std::path::PathBuf;
use std::env;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let proto_dir = manifest_dir.parent().unwrap().parent().unwrap().join("proto");

    // Only set PROTOC if the env var isn't already set and we can find protoc
    if env::var("PROTOC").is_err() {
        // Try common protoc locations
        let candidates = [
            "/usr/bin/protoc",
            "/usr/local/bin/protoc",
        ];
        for c in &candidates {
            if std::path::Path::new(c).exists() {
                env::set_var("PROTOC", c);
                break;
            }
        }
    }

    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        .compile_protos(
            &[
                proto_dir.join("enrollment.proto"),
                proto_dir.join("control.proto"),
                proto_dir.join("config.proto"),
                proto_dir.join("probe.proto"),
                proto_dir.join("artifact.proto"),
                proto_dir.join("update.proto"),
            ],
            &[&proto_dir],
        )?;

    Ok(())
}
