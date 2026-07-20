use std::path::PathBuf;
use std::env;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let proto_dir = manifest_dir.parent().unwrap().parent().unwrap().join("proto");

    let local_app_data = env::var("LOCALAPPDATA").unwrap_or_default();
    let protoc = PathBuf::from(&local_app_data)
        .join("protoc")
        .join("bin")
        .join("protoc.exe");

    env::set_var("PROTOC", protoc.to_string_lossy().replace("\\", "/"));

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
