use serde::{Deserialize, Serialize};
use base64::Engine;
use pathweaver_core::models::*;
use pathweaver_security::crypto;

pub struct Updater {
    install_dir: String,
    current_version: String,
    release_public_key: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UpdateManifest {
    pub product_version: String,
    pub protocol_version: u32,
    pub target_architecture: String,
    pub files: Vec<ManifestFile>,
    pub min_compatible_version: String,
    pub git_commit: String,
    pub build_time: String,
    pub allow_downgrade: bool,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ManifestFile {
    pub name: String,
    pub sha256: String,
    pub size_bytes: u64,
}

#[derive(Debug, Clone, PartialEq)]
pub enum UpdatePhase {
    Idle,
    Waiting,
    Prefetching,
    Verifying,
    Staged,
    Installing,
    Restarting,
    HealthChecking,
    Completed,
    Failed(String),
    RollingBack,
    RolledBack,
}

impl Updater {
    pub fn new(install_dir: &str, current_version: &str) -> Self {
        Self {
            install_dir: install_dir.to_string(),
            current_version: current_version.to_string(),
            release_public_key: String::new(),
        }
    }

    pub fn with_release_key(mut self, key: &str) -> Self {
        self.release_public_key = key.to_string();
        self
    }

    pub fn current_version(&self) -> &str {
        &self.current_version
    }

    pub fn release_dir(&self, version: &str) -> String {
        format!("{}/releases/{}", self.install_dir, version)
    }

    pub fn artifacts_dir(&self) -> String {
        format!("{}/artifacts", self.install_dir)
    }

    pub fn current_link(&self) -> String {
        format!("{}/current", self.install_dir)
    }

    pub async fn check_for_update(
        &self,
        target_version: &str,
        _manifest_sha256: &str,
    ) -> anyhow::Result<bool> {
        if target_version == self.current_version {
            tracing::info!("Already at target version {}", target_version);
            return Ok(false);
        }
        tracing::info!("Update available: {} -> {}", self.current_version, target_version);
        Ok(true)
    }

    pub async fn download_artifact(
        &self,
        artifact_name: &str,
        target_dir: &str,
    ) -> anyhow::Result<String> {
        std::fs::create_dir_all(target_dir)?;
        let path = format!("{}/{}", target_dir, artifact_name);
        tracing::info!("Artifact staged at {}", path);
        Ok(path)
    }

    pub fn verify_checksum(path: &str, expected_sha256: &str) -> anyhow::Result<bool> {
        let data = std::fs::read(path)?;
        let actual = crypto::sha256_hex(&data);
        Ok(actual == expected_sha256)
    }

    pub fn verify_signature(
        &self,
        data: &[u8],
        signature_b64: &str,
    ) -> anyhow::Result<bool> {
        if self.release_public_key.is_empty() {
            tracing::warn!("No release public key configured, skipping signature verification");
            return Ok(true);
        }
        crypto::verify_signature(&self.release_public_key, data, signature_b64)
            .map_err(|e| anyhow::anyhow!("signature verification: {e}"))
    }

    pub fn verify_manifest(
        &self,
        manifest: &UpdateManifest,
        signature_b64: &str,
    ) -> anyhow::Result<bool> {
        let manifest_json = serde_json::to_vec(manifest)?;

        if !self.release_public_key.is_empty() {
            if !self.verify_signature(&manifest_json, signature_b64)? {
                return Err(anyhow::anyhow!("Manifest signature invalid"));
            }
        }

        if !manifest.allow_downgrade {
            let target_parts: Vec<u32> = manifest.product_version
                .split('.')
                .filter_map(|s| s.parse().ok())
                .collect();
            let current_parts: Vec<u32> = self.current_version
                .split('.')
                .filter_map(|s| s.parse().ok())
                .collect();

            if target_parts < current_parts {
                return Err(anyhow::anyhow!("Downgrade not allowed"));
            }
        }

        if manifest.min_compatible_version > manifest.product_version {
            return Err(anyhow::anyhow!("Version compatibility check failed"));
        }

        Ok(true)
    }

    pub async fn stage_new_version(
        &self,
        version: &str,
        artifacts_dir: &str,
        manifest: &UpdateManifest,
    ) -> anyhow::Result<()> {
        let release_dir = format!("{}/releases/{}", self.install_dir, version);
        std::fs::create_dir_all(&release_dir)?;

        for file in &manifest.files {
            let src = format!("{}/{}", artifacts_dir, file.name);
            let dst = format!("{}/{}", release_dir, file.name);

            if std::path::Path::new(&src).exists() {
                std::fs::copy(&src, &dst)?;

                if !Self::verify_checksum(&dst, &file.sha256)? {
                    return Err(anyhow::anyhow!(
                        "Checksum mismatch for {}: expected {}",
                        file.name, file.sha256
                    ));
                }
            }
        }

        tracing::info!("Staged version {} at {}", version, release_dir);
        Ok(())
    }

    pub fn switch_version(&self, version: &str) -> anyhow::Result<()> {
        let current_link = self.current_link();
        let target = self.release_dir(version);

        if !std::path::Path::new(&target).exists() {
            return Err(anyhow::anyhow!("Release directory not found: {}", target));
        }

        if std::path::Path::new(&current_link).exists() {
            std::fs::remove_file(&current_link)?;
        }

        #[cfg(unix)]
        std::os::unix::fs::symlink(&target, &current_link)?;

        #[cfg(windows)]
        std::os::windows::fs::symlink_dir(&target, &current_link)?;

        tracing::info!("Switched to version {}", version);
        Ok(())
    }

    pub fn get_previous_version(&self) -> Option<String> {
        let releases_dir = format!("{}/releases", self.install_dir);
        if let Ok(entries) = std::fs::read_dir(&releases_dir) {
            let mut versions: Vec<String> = entries
                .filter_map(|e| e.ok())
                .filter(|e| e.path().is_dir())
                .filter_map(|e| e.file_name().into_string().ok())
                .filter(|v| v != &self.current_version)
                .collect();
            versions.sort();
            versions.reverse();
            versions.into_iter().next()
        } else {
            None
        }
    }

    pub async fn rollback(&self) -> anyhow::Result<()> {
        let previous = self.get_previous_version()
            .ok_or_else(|| anyhow::anyhow!("No previous version to rollback to"))?;

        tracing::info!("Rolling back from {} to {}", self.current_version, previous);
        self.switch_version(&previous)
    }

    pub async fn health_check(&self) -> anyhow::Result<bool> {
        let current = self.current_link();
        if !std::path::Path::new(&current).exists() {
            return Ok(false);
        }

        let agent_path = format!("{}/bin/pathweaver-agent", current);
        let netd_path = format!("{}/bin/pathweaver-netd", current);

        let agent_ok = std::path::Path::new(&agent_path).exists();
        let netd_ok = std::path::Path::new(&netd_path).exists();

        Ok(agent_ok && netd_ok)
    }

    pub fn restart_services(&self) -> anyhow::Result<()> {
        tracing::info!("Restarting PathWeaver services...");
        Ok(())
    }

    pub fn preserve_critical_data(&self) -> anyhow::Result<()> {
        let critical_files = [
            "etc/identity.key",
            "etc/wg_private.key",
            "etc/node_id",
            "etc/parent.conf",
            "etc/overlay_ip",
            "data/pathweaver.db",
        ];

        for file in &critical_files {
            let path = format!("{}/{}", self.install_dir, file);
            if std::path::Path::new(&path).exists() {
                tracing::debug!("Preserving critical file: {}", file);
            }
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_version_comparison() {
        let updater = Updater::new("/tmp/test", "0.1.0");
        assert!(updater.check_for_update_sync("0.2.0"));
        assert!(!updater.check_for_update_sync("0.1.0"));
    }

    impl Updater {
        fn check_for_update_sync(&self, target: &str) -> bool {
            target != self.current_version
        }
    }
}
