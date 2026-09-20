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
