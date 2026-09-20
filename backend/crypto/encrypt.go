package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	SaltSize       = 32
	IVSize         = 12 // GCM standard nonce size
	AuthTagSize    = 16 // GCM tag appended by Seal
	PBKDF2Iter     = 100_000
	KeySize        = 32 // AES-256
)

// DeriveKey produces a 256-bit key from the master key and a random salt using PBKDF2-SHA256.
func DeriveKey(masterKey []byte, salt []byte) []byte {
	return pbkdf2.Key(masterKey, salt, PBKDF2Iter, KeySize, sha256.New)
}

// GenerateSalt returns a cryptographically random salt.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	return salt, nil
}

// GenerateIV returns a cryptographically random initialization vector.
func GenerateIV() ([]byte, error) {
	iv := make([]byte, IVSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("generate IV: %w", err)
	}
	return iv, nil
}

// EncryptChunk encrypts plaintext using AES-256-GCM with the given derived key and IV.
// Returns: ciphertext with appended auth tag.
func EncryptChunk(derivedKey, iv, plaintext []byte) (ciphertext []byte, authTag []byte, err error) {
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("create GCM: %w", err)
	}

	// Seal appends the auth tag to the ciphertext
	sealed := gcm.Seal(nil, iv, plaintext, nil)

	// Split ciphertext and auth tag
	ct := sealed[:len(sealed)-AuthTagSize]
	tag := sealed[len(sealed)-AuthTagSize:]

	return ct, tag, nil
}

// DecryptChunk decrypts ciphertext using AES-256-GCM with the given derived key, IV, and auth tag.
func DecryptChunk(derivedKey, iv, ciphertext, authTag []byte) ([]byte, error) {
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	// Rejoin ciphertext + auth tag for Open
	sealed := append(ciphertext, authTag...)

	plaintext, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

// HexEncode converts bytes to hex string for storage.
func HexEncode(data []byte) string {
	return hex.EncodeToString(data)
}

// HexDecode converts hex string back to bytes.
func HexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
