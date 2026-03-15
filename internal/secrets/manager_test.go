package secrets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/hkdf"
)

func TestNewMasterKeyManager_FromEnvVar_HKDF(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMasterKeyManager("my-production-key-here", dir)
	if err != nil {
		t.Fatalf("NewMasterKeyManager: %v", err)
	}

	// Verify key is derived via HKDF
	reader := hkdf.New(sha256.New, []byte("my-production-key-here"), []byte(hkdfSalt), []byte(hkdfInfo))
	expected := make([]byte, keySize)
	if _, err := io.ReadFull(reader, expected); err != nil {
		t.Fatalf("HKDF: %v", err)
	}

	key := m.Key()
	if !bytes.Equal(key[:], expected) {
		t.Error("key does not match expected HKDF derivation")
	}
}

func TestNewMasterKeyManager_FromEnvVar_RawHex(t *testing.T) {
	rawKey := make([]byte, 32)
	for i := range rawKey {
		rawKey[i] = byte(i)
	}
	hexKey := hex.EncodeToString(rawKey)

	dir := t.TempDir()
	m, err := NewMasterKeyManager(hexKey, dir)
	if err != nil {
		t.Fatalf("NewMasterKeyManager: %v", err)
	}

	key := m.Key()
	if !bytes.Equal(key[:], rawKey) {
		t.Error("key does not match raw hex input")
	}
}

func TestNewMasterKeyManager_AutoGenerate(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMasterKeyManager("", dir)
	if err != nil {
		t.Fatalf("NewMasterKeyManager: %v", err)
	}

	// Verify key file exists
	keyPath := filepath.Join(dir, keyFileName)
	data, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("reading key file: %v", err)
	}
	if len(data) != keySize {
		t.Fatalf("key file length = %d, want %d", len(data), keySize)
	}

	key := m.Key()
	if !bytes.Equal(key[:], data) {
		t.Error("in-memory key does not match file")
	}

	// Verify permissions
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("key file permissions = %04o, want 0600", perm)
	}
}

func TestNewMasterKeyManager_LoadFromFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFileName)

	// Write a known key
	knownKey := make([]byte, keySize)
	for i := range knownKey {
		knownKey[i] = byte(i + 100)
	}
	if err := os.WriteFile(keyPath, knownKey, 0600); err != nil {
		t.Fatalf("writing key file: %v", err)
	}

	m, err := NewMasterKeyManager("", dir)
	if err != nil {
		t.Fatalf("NewMasterKeyManager: %v", err)
	}

	key := m.Key()
	if !bytes.Equal(key[:], knownKey) {
		t.Error("key does not match file content")
	}
}

func TestNewMasterKeyManager_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFileName)

	// Write a file with wrong length
	if err := os.WriteFile(keyPath, make([]byte, 16), 0600); err != nil {
		t.Fatalf("writing key file: %v", err)
	}

	_, err := NewMasterKeyManager("", dir)
	if err == nil {
		t.Fatal("expected error for invalid key file length")
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	m, err := NewMasterKeyManager("test-key-for-roundtrip", t.TempDir())
	if err != nil {
		t.Fatalf("NewMasterKeyManager: %v", err)
	}

	plaintext := []byte("my-super-secret-value")
	ciphertext, nonce, err := m.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := m.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_DifferentNonces(t *testing.T) {
	m, err := NewMasterKeyManager("test-key-nonces", t.TempDir())
	if err != nil {
		t.Fatalf("NewMasterKeyManager: %v", err)
	}

	plaintext := []byte("same-plaintext")
	ct1, n1, _ := m.Encrypt(plaintext)
	ct2, n2, _ := m.Encrypt(plaintext)

	if bytes.Equal(n1, n2) {
		t.Error("nonces should differ between encryptions")
	}
	if bytes.Equal(ct1, ct2) {
		t.Error("ciphertexts should differ between encryptions")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	m1, _ := NewMasterKeyManager("key-one", t.TempDir())
	m2, _ := NewMasterKeyManager("key-two", t.TempDir())

	ct, nonce, _ := m1.Encrypt([]byte("secret"))
	_, err := m2.Decrypt(ct, nonce)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	m, _ := NewMasterKeyManager("tamper-test", t.TempDir())

	ct, nonce, _ := m.Encrypt([]byte("secret"))
	ct[0] ^= 0xFF // tamper

	_, err := m.Decrypt(ct, nonce)
	if err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	m, _ := NewMasterKeyManager("empty-test", t.TempDir())

	ct, nonce, err := m.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	decrypted, err := m.Decrypt(ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("expected empty decryption, got %d bytes", len(decrypted))
	}
}

func TestKeyDerivation_Deterministic(t *testing.T) {
	m1, _ := NewMasterKeyManager("deterministic-test", t.TempDir())
	m2, _ := NewMasterKeyManager("deterministic-test", t.TempDir())

	if m1.Key() != m2.Key() {
		t.Error("same input should produce same key")
	}
}
