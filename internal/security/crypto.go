package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/curve25519"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	// Encode: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, b64Salt, b64Hash), nil
}

func VerifyPassword(encoded, password string) bool {
	var memory, timeCost uint32
	var threads uint8
	var saltB64, hashB64 string
	_, err := fmt.Sscanf(encoded, "$argon2id$v=19$m=%d,t=%d,p=%d$", &memory, &timeCost, &threads)
	if err != nil {
		return false
	}
	parts := splitDollar(encoded)
	if len(parts) < 6 {
		return false
	}
	saltB64, hashB64 = parts[4], parts[5]
	salt, err := base64.RawStdEncoding.DecodeString(saltB64)
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(hashB64)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))
	return hmac.Equal(got, want)
}

func splitDollar(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '$' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SecretBox encrypts secrets at rest with AES-GCM.
type SecretBox struct {
	gcm cipher.AEAD
}

func LoadOrCreateKey(path string) (*SecretBox, error) {
	var key []byte
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		key = data[:32]
	} else {
		key = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, err
		}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{gcm: gcm}, nil
}

func (s *SecretBox) Encrypt(plain string) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := s.gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func (s *SecretBox) Decrypt(cipherB64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		return "", err
	}
	ns := s.gcm.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := s.gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// GenerateWGKeyPair returns (privateKey base64, publicKey base64) WireGuard-style.
func GenerateWGKeyPair() (privB64, pubB64 string, err error) {
	var priv [32]byte
	if _, err = io.ReadFull(rand.Reader, priv[:]); err != nil {
		return "", "", err
	}
	clampWG(&priv)
	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)
	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:]), nil
}

func clampWG(k *[32]byte) {
	k[0] &= 248
	k[31] &= 127
	k[31] |= 64
}

func GenerateIdentityKeyPair() (privB64, pubB64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(pub), nil
}

func SignEd25519(privB64 string, msg []byte) (string, error) {
	priv, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		return "", err
	}
	if len(priv) != ed25519.PrivateKeySize {
		return "", errors.New("invalid private key size")
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), msg)
	return base64.StdEncoding.EncodeToString(sig), nil
}

func VerifyEd25519(pubB64, sigB64 string, msg []byte) bool {
	pub, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}

func HMACSHA256(key []byte, msg string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RateLimiter is a simple sliding-window limiter.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{attempts: make(map[string][]time.Time), max: max, window: window}
}

func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cut := now.Add(-r.window)
	var kept []time.Time
	for _, t := range r.attempts[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.max {
		r.attempts[key] = kept
		return false
	}
	kept = append(kept, now)
	r.attempts[key] = kept
	return true
}
