// Package secrets provides master key management and AES-256-GCM encryption for squad secrets.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/hkdf"
)

const (
	// nonceSize is the standard GCM nonce length.
	nonceSize = 12
	// keySize is the AES-256 key length.
	keySize = 32
	// keyFileName is the auto-generated key file name.
	keyFileName = "master.key"
	// hkdfSalt is the salt for HKDF key derivation.
	hkdfSalt = "ari-master-key-v1"
	// hkdfInfo is the info parameter for HKDF key derivation.
	hkdfInfo = "ari-secrets-encryption"
)

// MasterKeyManager handles loading, generating, and rotating the master encryption key.
type MasterKeyManager struct {
	key     [keySize]byte
	dataDir string
	mu      sync.RWMutex
}

// NewMasterKeyManager creates a MasterKeyManager following the initialization order:
// 1. If masterKeyEnv is exactly 64 hex chars, decode as raw 256-bit key.
// 2. If masterKeyEnv is non-empty (other format), derive via HKDF(SHA-256).
// 3. If masterKeyEnv is empty, check for {dataDir}/master.key file.
// 4. If no file, auto-generate a random key and write to {dataDir}/master.key.
func NewMasterKeyManager(masterKeyEnv string, dataDir string) (*MasterKeyManager, error) {
	m := &MasterKeyManager{dataDir: dataDir}

	if masterKeyEnv != "" {
		// Try raw hex first
		if len(masterKeyEnv) == 64 {
			decoded, err := hex.DecodeString(masterKeyEnv)
			if err == nil && len(decoded) == keySize {
				copy(m.key[:], decoded)
				slog.Info("master key loaded from env var (raw hex)")
				return m, nil
			}
			// If hex decode fails, fall through to HKDF
		}

		// Derive via HKDF
		if err := m.deriveKey(masterKeyEnv); err != nil {
			return nil, fmt.Errorf("deriving master key: %w", err)
		}
		slog.Info("master key derived from env var via HKDF")
		return m, nil
	}

	// Check for existing key file
	keyPath := filepath.Join(dataDir, keyFileName)
	if data, err := os.ReadFile(keyPath); err == nil {
		if len(data) != keySize {
			return nil, fmt.Errorf("master key file %s has invalid length %d (expected %d)", keyPath, len(data), keySize)
		}
		copy(m.key[:], data)

		// Check file permissions and warn if too permissive
		info, err := os.Stat(keyPath)
		if err == nil {
			perm := info.Mode().Perm()
			if perm&0077 != 0 {
				slog.Warn("master.key has permissive file permissions",
					"path", keyPath,
					"permissions", fmt.Sprintf("%04o", perm),
					"expected", "0600")
			}
		}

		slog.Info("master key loaded from file", "path", keyPath)
		return m, nil
	}

	// Auto-generate
	if _, err := rand.Read(m.key[:]); err != nil {
		return nil, fmt.Errorf("generating master key: %w", err)
	}

	// Ensure data dir exists
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	if err := os.WriteFile(keyPath, m.key[:], 0600); err != nil {
		return nil, fmt.Errorf("writing master key file: %w", err)
	}

	slog.Warn("auto-generated master key — set ARI_MASTER_KEY for production", "path", keyPath)
	return m, nil
}

// deriveKey uses HKDF with SHA-256 to derive the encryption key from the input.
func (m *MasterKeyManager) deriveKey(input string) error {
	reader := hkdf.New(sha256.New, []byte(input), []byte(hkdfSalt), []byte(hkdfInfo))
	_, err := io.ReadFull(reader, m.key[:])
	return err
}

// Encrypt encrypts plaintext using AES-256-GCM with a random 12-byte nonce.
func (m *MasterKeyManager) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	m.mu.RLock()
	key := m.key
	m.mu.RUnlock()

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce = make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with the provided nonce.
func (m *MasterKeyManager) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	m.mu.RLock()
	key := m.key
	m.mu.RUnlock()

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}

// RotateKey generates a new random key, atomically swaps it, and returns the old key.
// The caller is responsible for re-encrypting data and persisting the new key.
func (m *MasterKeyManager) RotateKey() (oldKey [keySize]byte, err error) {
	var newKey [keySize]byte
	if _, err := rand.Read(newKey[:]); err != nil {
		return oldKey, fmt.Errorf("generating new key: %w", err)
	}

	m.mu.Lock()
	oldKey = m.key
	m.key = newKey
	m.mu.Unlock()

	return oldKey, nil
}

// RestoreKey restores a previously saved key (used on rotation rollback).
func (m *MasterKeyManager) RestoreKey(key [keySize]byte) {
	m.mu.Lock()
	m.key = key
	m.mu.Unlock()
}

// PersistKey writes the current key to {dataDir}/master.key with 0600 permissions.
func (m *MasterKeyManager) PersistKey() error {
	m.mu.RLock()
	key := m.key
	m.mu.RUnlock()

	keyPath := filepath.Join(m.dataDir, keyFileName)
	return os.WriteFile(keyPath, key[:], 0600)
}

// Key returns the current key (for testing only).
func (m *MasterKeyManager) Key() [keySize]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.key
}

// DecryptWithKey decrypts using a specific key (for re-encryption during rotation).
func DecryptWithKey(key [keySize]byte, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}
