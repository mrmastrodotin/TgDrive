package telegram

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Client wraps the Telegram Bot API for high-throughput file storage operations.
type Client struct {
	bot        *bot.Bot
	channelID  int64
	httpClient *http.Client
}

// NewClient creates a new Telegram client with an optimized HTTP connection pool.
func NewClient(token string, channelID int64) (*Client, error) {
	// Custom HTTP transport with high connection pooling
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,  // Encrypted blobs cannot be compressed
		ForceAttemptHTTP2:   false, // HTTP/2 causes PROTOCOL_ERROR with Telegram CDN
		TLSNextProto:        make(map[string]func(authority string, c *tls.Conn) http.RoundTripper), // Force HTTP/1.1
		TLSClientConfig:     &tls.Config{
			NextProtos: []string{"http/1.1"},
		},
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Minute,
	}

	b, err := bot.New(token, bot.WithHTTPClient(10*time.Minute, httpClient))
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	return &Client{
		bot:        b,
		channelID:  channelID,
		httpClient: httpClient,
	}, nil
}

// VerifyAccess checks that the bot can post to the configured channel.
func (c *Client) VerifyAccess(ctx context.Context) error {
	_, err := c.bot.GetChat(ctx, &bot.GetChatParams{
		ChatID: c.channelID,
	})
	if err != nil {
		return fmt.Errorf("cannot access channel %d: %w", c.channelID, err)
	}
	return nil
}

// UploadResult holds the Telegram response after uploading a chunk.
type UploadResult struct {
	MessageID int
	FileID    string
}

// UploadChunk sends an encrypted chunk as a document to the channel.
func (c *Client) UploadChunk(ctx context.Context, data []byte, filename string) (*UploadResult, error) {
	reader := bytes.NewReader(data)

	params := &bot.SendDocumentParams{
		ChatID: c.channelID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     reader,
		},
		DisableNotification: true,
	}

	msg, err := c.bot.SendDocument(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("upload chunk: %w", err)
	}

	if msg.Document == nil {
		return nil, fmt.Errorf("upload chunk: no document in response")
	}

	return &UploadResult{
		MessageID: msg.ID,
		FileID:    msg.Document.FileID,
	}, nil
}

// DownloadChunk fetches an encrypted chunk from Telegram by file_id using the pooled HTTP client.
func (c *Client) DownloadChunk(ctx context.Context, fileID string) ([]byte, error) {
	file, err := c.bot.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		return nil, fmt.Errorf("get file info: %w", err)
	}

	if file.FilePath == "" {
		return nil, fmt.Errorf("file path empty for file_id %s", fileID)
	}

	// Build download URL
	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s",
		c.bot.Token(), file.FilePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download chunk: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download chunk: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chunk body: %w", err)
	}

	return data, nil
}

// DeleteMessage deletes a message from the channel (to remove an uploaded chunk).
func (c *Client) DeleteMessage(ctx context.Context, messageID int) error {
	_, err := c.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    c.channelID,
		MessageID: messageID,
	})
	return err
}
