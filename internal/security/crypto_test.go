package security_test

import (
	"testing"

	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
)

func TestPasswordHash(t *testing.T) {
	h, err := security.HashPassword("secret-password")
	if err != nil {
		t.Fatal(err)
	}
	if !security.VerifyPassword(h, "secret-password") {
		t.Fatal("verify failed")
	}
	if security.VerifyPassword(h, "wrong") {
		t.Fatal("should fail")
	}
}

func TestWGKeys(t *testing.T) {
	priv, pub, err := security.GenerateWGKeyPair()
	if err != nil || priv == "" || pub == "" {
		t.Fatal(err)
	}
}

func TestSecretBox(t *testing.T) {
	dir := t.TempDir()
	box, err := security.LoadOrCreateKey(dir + "/key")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := box.Encrypt("hello")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := box.Decrypt(enc)
	if err != nil || plain != "hello" {
		t.Fatalf("got %q %v", plain, err)
	}
}
