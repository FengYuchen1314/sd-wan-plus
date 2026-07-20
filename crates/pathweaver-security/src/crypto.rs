use ed25519_dalek::{SigningKey, Signer, Verifier, VerifyingKey, Signature};
use sha2::{Sha256, Digest};
use rand::rngs::OsRng;
use base64::Engine;
use pathweaver_core::error::{PathWeaverError, Result};

pub fn generate_identity_keypair() -> (String, String) {
    let signing_key = SigningKey::generate(&mut OsRng);
    let verifying_key = signing_key.verifying_key();

    let private = hex::encode(signing_key.to_bytes());
    let public = hex::encode(verifying_key.to_bytes());

    (private, public)
}

pub fn generate_wireguard_keypair() -> (String, String) {
    let signing_key = SigningKey::generate(&mut OsRng);
    let verifying_key = signing_key.verifying_key();

    let private = base64::engine::general_purpose::STANDARD.encode(signing_key.to_bytes());
    let public = base64::engine::general_purpose::STANDARD.encode(verifying_key.to_bytes());

    (private, public)
}

pub fn sign_data(private_key_hex: &str, data: &[u8]) -> Result<String> {
    let key_bytes = hex::decode(private_key_hex)
        .map_err(|e| PathWeaverError::InternalError(format!("invalid private key: {e}")))?;

    let arr: [u8; 32] = key_bytes
        .try_into()
        .map_err(|_| PathWeaverError::InternalError("invalid key length".into()))?;

    let signing_key = SigningKey::from_bytes(&arr);
    let signature = signing_key.sign(data);

    Ok(base64::engine::general_purpose::STANDARD.encode(signature.to_bytes()))
}

pub fn verify_signature(public_key_hex: &str, data: &[u8], signature_b64: &str) -> Result<bool> {
    let key_bytes = hex::decode(public_key_hex)
        .map_err(|e| PathWeaverError::InternalError(format!("invalid public key: {e}")))?;

    let arr: [u8; 32] = key_bytes
        .try_into()
        .map_err(|_| PathWeaverError::InternalError("invalid key length".into()))?;

    let verifying_key = VerifyingKey::from_bytes(&arr)
        .map_err(|e| PathWeaverError::InternalError(format!("invalid public key: {e}")))?;

    let sig_bytes = base64::engine::general_purpose::STANDARD
        .decode(signature_b64)
        .map_err(|e| PathWeaverError::InternalError(format!("invalid signature: {e}")))?;

    let sig_arr: [u8; 64] = sig_bytes
        .try_into()
        .map_err(|_| PathWeaverError::InternalError("invalid signature length".into()))?;

    let signature = Signature::from_bytes(&sig_arr);

    Ok(verifying_key.verify(data, &signature).is_ok())
}

pub fn sha256_hex(data: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(data);
    hex::encode(hasher.finalize())
}

pub fn sha256_base64(data: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(data);
    base64::engine::general_purpose::STANDARD.encode(hasher.finalize())
}

pub fn generate_random_token(length: usize) -> String {
    use rand::RngCore;
    let mut bytes = vec![0u8; length];
    OsRng.fill_bytes(&mut bytes);
    hex::encode(&bytes)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_identity_keypair_sign_verify() {
        let (private, public) = generate_identity_keypair();
        let data = b"test data to sign";

        let sig = sign_data(&private, data).unwrap();
        assert!(verify_signature(&public, data, &sig).unwrap());

        let bad_data = b"different data";
        assert!(!verify_signature(&public, bad_data, &sig).unwrap());
    }

    #[test]
    fn test_sha256() {
        let hash = sha256_hex(b"hello world");
        assert_eq!(hash.len(), 64);
    }

    #[test]
    fn test_generate_token() {
        let token1 = generate_random_token(32);
        let token2 = generate_random_token(32);
        assert_ne!(token1, token2);
        assert_eq!(token1.len(), 64);
    }
}
