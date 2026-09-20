package crypto

import (
	"fmt"
	"io"
	"sync"
)

const (
	// ChunkSize is 20MB raw data per chunk before encryption.
	ChunkSize = 20 * 1024 * 1024
	// PrefetchWorkers defines how many chunks to download concurrently in advance
	PrefetchWorkers = 3
)

// EncryptedChunk holds one encrypted block and its associated crypto metadata.
type EncryptedChunk struct {
	Index   int
	Data    []byte // Encrypted payload (ciphertext only, no tag)
	IV      []byte
	AuthTag []byte
	RawSize int // Pre-encryption plaintext size
}

// ChunkAndEncrypt reads from src, splits into ChunkSize blocks, and encrypts each independently.
// All chunks share the same derived key (from master key + salt), but each gets a unique IV.
func ChunkAndEncrypt(src io.Reader, masterKey, salt []byte) ([]EncryptedChunk, error) {
	derivedKey := DeriveKey(masterKey, salt)

	var chunks []EncryptedChunk
	buf := make([]byte, ChunkSize)
	index := 0

	for {
		n, err := io.ReadFull(src, buf)
		if n == 0 {
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read chunk %d: %w", index, err)
			}
		}

		plaintext := buf[:n]

		iv, ivErr := GenerateIV()
		if ivErr != nil {
			return nil, fmt.Errorf("IV for chunk %d: %w", index, ivErr)
		}

		ciphertext, authTag, encErr := EncryptChunk(derivedKey, iv, plaintext)
		if encErr != nil {
			return nil, fmt.Errorf("encrypt chunk %d: %w", index, encErr)
		}

		chunks = append(chunks, EncryptedChunk{
			Index:   index,
			Data:    ciphertext,
			IV:      iv,
			AuthTag: authTag,
			RawSize: n,
		})

		index++

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	return chunks, nil
}

// DecryptSingleChunk decrypts a single chunk given the master key, salt, and chunk metadata.
func DecryptSingleChunk(encryptedData, masterKey, salt, iv, authTag []byte) ([]byte, error) {
	derivedKey := DeriveKey(masterKey, salt)
	return DecryptChunk(derivedKey, iv, encryptedData, authTag)
}

// ChunkMeta holds metadata needed to decrypt a chunk.
type ChunkMeta struct {
	Index   int
	IV      string // Hex-encoded
	AuthTag string // Hex-encoded
}

type fetchedChunk struct {
	data []byte
	err  error
}

// DecryptAndStream decrypts chunks in order and writes plaintext to dst.
// Uses a concurrent pipelined prefetch queue so future chunks download in parallel while current chunk streams.
func DecryptAndStream(
	dst io.Writer,
	masterKey, salt []byte,
	chunkMetas []ChunkMeta,
	fetchChunk func(index int) ([]byte, error),
) error {
	if len(chunkMetas) == 0 {
		return nil
	}

	derivedKey := DeriveKey(masterKey, salt)

	// Single chunk: direct fetch & decrypt without pipeline overhead
	if len(chunkMetas) == 1 {
		meta := chunkMetas[0]
		iv, err := HexDecode(meta.IV)
		if err != nil {
			return fmt.Errorf("decode IV for chunk %d: %w", meta.Index, err)
		}
		authTag, err := HexDecode(meta.AuthTag)
		if err != nil {
			return fmt.Errorf("decode auth tag for chunk %d: %w", meta.Index, err)
		}
		encData, err := fetchChunk(meta.Index)
		if err != nil {
			return fmt.Errorf("fetch chunk %d: %w", meta.Index, err)
		}
		plaintext, err := DecryptChunk(derivedKey, iv, encData, authTag)
		if err != nil {
			return fmt.Errorf("decrypt chunk %d: %w", meta.Index, err)
		}
		_, err = dst.Write(plaintext)
		return err
	}

	// Pipelined concurrent prefetching
	results := make(map[int]chan fetchedChunk)
	var mu sync.Mutex

	getOrCreateChan := func(idx int) chan fetchedChunk {
		mu.Lock()
		defer mu.Unlock()
		if ch, exists := results[idx]; exists {
			return ch
		}
		ch := make(chan fetchedChunk, 1)
		results[idx] = ch
		return ch
	}

	// Bounded semaphore to limit simultaneous active network fetches
	sem := make(chan struct{}, PrefetchWorkers)

	// Background worker coordinator
	go func() {
		for _, meta := range chunkMetas {
			idx := meta.Index
			resChan := getOrCreateChan(idx)

			sem <- struct{}{}
			go func(chunkIdx int, ch chan fetchedChunk) {
				defer func() { <-sem }()
				data, err := fetchChunk(chunkIdx)
				ch <- fetchedChunk{data: data, err: err}
			}(idx, resChan)
		}
	}()

	// Sequential consumer: decrypt & write in exact stream order
	for _, meta := range chunkMetas {
		iv, err := HexDecode(meta.IV)
		if err != nil {
			return fmt.Errorf("decode IV for chunk %d: %w", meta.Index, err)
		}

		authTag, err := HexDecode(meta.AuthTag)
		if err != nil {
			return fmt.Errorf("decode auth tag for chunk %d: %w", meta.Index, err)
		}

		resChan := getOrCreateChan(meta.Index)
		res := <-resChan
		if res.err != nil {
			return fmt.Errorf("fetch chunk %d: %w", meta.Index, res.err)
		}

		plaintext, err := DecryptChunk(derivedKey, iv, res.data, authTag)
		if err != nil {
			return fmt.Errorf("decrypt chunk %d: %w", meta.Index, err)
		}

		if _, err := dst.Write(plaintext); err != nil {
			return fmt.Errorf("write chunk %d: %w", meta.Index, err)
		}
	}

	return nil
}
