package crypto

import (
	"bytes"
	"testing"
)

func TestDeriveKeyAndEncryptDecrypt(t *testing.T) {
	masterKey := []byte("test_master_secret_key_123456789")
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	key := DeriveKey(masterKey, salt)
	if len(key) != KeySize {
		t.Fatalf("expected key length %d, got %d", KeySize, len(key))
	}

	iv, err := GenerateIV()
	if err != nil {
		t.Fatalf("GenerateIV failed: %v", err)
	}

	originalData := []byte("Hello TgDrive! Streaming encrypted content straight to Telegram.")
	ciphertext, authTag, err := EncryptChunk(key, iv, originalData)
	if err != nil {
		t.Fatalf("EncryptChunk failed: %v", err)
	}

	if bytes.Equal(originalData, ciphertext) {
		t.Fatal("encrypted data matches original plain text")
	}

	decryptedData, err := DecryptChunk(key, iv, ciphertext, authTag)
	if err != nil {
		t.Fatalf("DecryptChunk failed: %v", err)
	}

	if !bytes.Equal(originalData, decryptedData) {
		t.Fatalf("decrypted data %q != original %q", decryptedData, originalData)
	}
}

func TestChunkAndEncryptReassemble(t *testing.T) {
	masterKey := []byte("test_master_key_987654321")
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	// 50KB test payload
	testPayload := make([]byte, 50*1024)
	for i := range testPayload {
		testPayload[i] = byte(i % 251)
	}

	reader := bytes.NewReader(testPayload)
	chunks, err := ChunkAndEncrypt(reader, masterKey, salt)
	if err != nil {
		t.Fatalf("ChunkAndEncrypt failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	// Decrypt each chunk and verify reconstruction
	var reconstructed bytes.Buffer
	for _, ch := range chunks {
		plain, err := DecryptSingleChunk(ch.Data, masterKey, salt, ch.IV, ch.AuthTag)
		if err != nil {
			t.Fatalf("DecryptSingleChunk failed on chunk %d: %v", ch.Index, err)
		}
		reconstructed.Write(plain)
	}

	if !bytes.Equal(testPayload, reconstructed.Bytes()) {
		t.Fatal("reconstructed payload does not match original")
	}
}

func TestCredentialEncryptionAtRest(t *testing.T) {
	secret := "my_dedicated_bot_credential_secret_32bytes!"
	key := DeriveCredentialKey(secret)

	rawToken := "123456789:ABCdefGHIjklMNOpqrsTUVwxyz_123456"

	// 1. Encrypt token
	encrypted, err := EncryptCredential(key, rawToken)
	if err != nil {
		t.Fatalf("EncryptCredential failed: %v", err)
	}

	if encrypted == rawToken {
		t.Fatal("encrypted credential matches raw token")
	}

	// 2. Decrypt token
	decrypted, err := DecryptCredential(key, encrypted)
	if err != nil {
		t.Fatalf("DecryptCredential failed: %v", err)
	}

	if decrypted != rawToken {
		t.Fatalf("decrypted %s != expected %s", decrypted, rawToken)
	}

	// 3. Backward compatibility: plaintext legacy token should decrypt to itself
	legacyPlain := "987654321:LEGACY_PLAINTEXT_TOKEN"
	legacyDecrypted, err := DecryptCredential(key, legacyPlain)
	if err != nil {
		t.Fatalf("DecryptCredential on legacy plaintext failed: %v", err)
	}
	if legacyDecrypted != legacyPlain {
		t.Fatalf("expected legacy plaintext to be returned as-is, got %s", legacyDecrypted)
	}

	// 4. Tampered ciphertext should fail GCM authentication
	tampered := encrypted[:len(encrypted)-4] + "ffff"
	_, err = DecryptCredential(key, tampered)
	if err == nil {
		t.Fatal("expected authentication failure on tampered credential")
	}
}

