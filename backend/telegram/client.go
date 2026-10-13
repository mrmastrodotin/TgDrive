package telegram

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"tgdrive/crypto"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// BotPool manages a dynamic pool of Telegram bot clients for high-throughput parallel transfers.
type BotPool struct {
	workers               map[string]*botWorker
	workerList            []*botWorker
	hashAliases           map[string]string // Maps oldTokenHash -> newTokenHash for rotated tokens of the same bot
	ownerActive           map[string]int
	roundRobin            map[string]int
	channelID             int64
	serverURL             string
	globalMaxConcurrency int
	maxUserConcurrency   int
	maxPerBotConcurrency int
	globalActive          int
	cond                  *sync.Cond
	mu                    sync.Mutex
	transport             *http.Transport
}

type botWorker struct {
	token         string
	tokenHash     string
	ownerID       string
	bot           *bot.Bot
	httpClient    *http.Client
	cooldownUntil time.Time
	disabled      bool // true means disabled for new chunk uploads
	downloadOnly  bool // true means retained strictly to download existing chunks
	activeUploads int
	maxConcurrent int
}

// BotStatus represents public runtime status of a bot in the pool.
type BotStatus struct {
	TokenHash     string     `json:"token_hash"`
	OwnerID       string     `json:"owner_id"`
	Online        bool       `json:"online"`
	Disabled      bool       `json:"disabled"`
	ActiveUploads int        `json:"active_uploads"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`
}

// UploadResult holds the Telegram response after uploading a chunk.
type UploadResult struct {
	MessageID    int
	FileID       string
	BotTokenHash string
}

// NewBotPool creates a new Telegram client pool with optimized HTTP connection pools.
func NewBotPool(tokens []string, channelID int64, serverURL string) (*BotPool, error) {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,  // Encrypted blobs cannot be compressed
		ForceAttemptHTTP2:   false, // HTTP/2 causes PROTOCOL_ERROR with Telegram CDN
		TLSNextProto:        make(map[string]func(authority string, c *tls.Conn) http.RoundTripper), // Force HTTP/1.1
		TLSClientConfig: &tls.Config{
			NextProtos: []string{"http/1.1"},
		},
	}

	pool := &BotPool{
		workers:               make(map[string]*botWorker),
		hashAliases:           make(map[string]string),
		ownerActive:           make(map[string]int),
		roundRobin:            make(map[string]int),
		channelID:             channelID,
		serverURL:             serverURL,
		transport:             transport,
		maxPerBotConcurrency: 1, // 1 active chunk upload per bot as per spec
		globalMaxConcurrency: len(tokens),
		maxUserConcurrency:   6,
	}
	pool.cond = sync.NewCond(&pool.mu)

	for _, token := range tokens {
		if err := pool.addBotInternal("admin", token); err != nil {
			return nil, err
		}
	}

	return pool, nil
}

// SetMaxUserConcurrency configures the maximum concurrent chunk operations for any single user.
func (p *BotPool) SetMaxUserConcurrency(limit int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if limit > 0 {
		p.maxUserConcurrency = limit
	}
}

func (p *BotPool) addBotInternal(ownerID, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if ownerID == "" {
		ownerID = "admin"
	}

	httpClient := &http.Client{
		Transport: p.transport,
		Timeout:   10 * time.Minute,
	}

	opts := []bot.Option{
		bot.WithHTTPClient(10*time.Minute, httpClient),
		bot.WithSkipGetMe(),
	}
	if p.serverURL != "" {
		opts = append(opts, bot.WithServerURL(p.serverURL))
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
	}

	hash := crypto.HashToken(token)

	worker := &botWorker{
		token:         token,
		tokenHash:     hash,
		ownerID:       ownerID,
		bot:           b,
		httpClient:    httpClient,
		maxConcurrent: p.maxPerBotConcurrency,
	}

	p.workers[hash] = worker
	p.workerList = append(p.workerList, worker)
	return nil
}

// AddBot dynamically registers a new Telegram bot into the live worker pool.
// Supports either (token) which defaults to owner "admin", or (ownerID, token).
// Enforces that ownership of any token hash is immutable.
func (p *BotPool) AddBot(args ...string) error {
	var ownerID, token string
	if len(args) == 1 {
		ownerID = "admin"
		token = args[0]
	} else if len(args) >= 2 {
		ownerID = args[0]
		token = args[1]
	} else {
		return fmt.Errorf("token cannot be empty")
	}
	if ownerID == "" {
		ownerID = "admin"
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	hash := crypto.HashToken(token)

	p.mu.Lock()
	if existing, exists := p.workers[hash]; exists {
		if existing.ownerID != ownerID {
			p.mu.Unlock()
			return fmt.Errorf("bot already owned by another user (%s)", existing.ownerID)
		}
		if existing.disabled {
			existing.disabled = false
			p.cond.Broadcast()
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		return fmt.Errorf("bot already exists in pool")
	}
	p.mu.Unlock()

	httpClient := &http.Client{
		Transport: p.transport,
		Timeout:   10 * time.Minute,
	}

	opts := []bot.Option{
		bot.WithHTTPClient(10*time.Minute, httpClient),
		bot.WithSkipGetMe(),
	}
	if p.serverURL != "" {
		opts = append(opts, bot.WithServerURL(p.serverURL))
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
	}

	worker := &botWorker{
		token:         token,
		tokenHash:     hash,
		ownerID:       ownerID,
		bot:           b,
		httpClient:    httpClient,
		maxConcurrent: p.maxPerBotConcurrency,
	}

	p.mu.Lock()
	p.workers[hash] = worker
	p.workerList = append(p.workerList, worker)
	if p.globalMaxConcurrency < len(p.workerList) {
		p.globalMaxConcurrency = len(p.workerList)
	}
	p.cond.Broadcast()
	p.mu.Unlock()

	return nil
}

// RemoveBot dynamically removes a bot worker from the pool, strictly verifying ownership.
// Supports either (tokenHash) which defaults to owner "admin", or (ownerID, tokenHash).
func (p *BotPool) RemoveBot(args ...string) error {
	var ownerID, tokenHash string
	if len(args) == 1 {
		ownerID = "admin"
		tokenHash = args[0]
	} else if len(args) >= 2 {
		ownerID = args[0]
		tokenHash = args[1]
	} else {
		return fmt.Errorf("token hash required")
	}
	if ownerID == "" {
		ownerID = "admin"
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	worker, exists := p.workers[tokenHash]
	if !exists {
		return fmt.Errorf("bot worker with hash %s not found", tokenHash)
	}

	if worker.ownerID != ownerID && ownerID != "admin" {
		return fmt.Errorf("unauthorized: bot %s does not belong to owner %s", tokenHash, ownerID)
	}

	// Check if this is the last bot for this owner
	ownerBotCount := 0
	for _, w := range p.workerList {
		if w.ownerID == worker.ownerID && !w.disabled {
			ownerBotCount++
		}
	}
	if ownerBotCount <= 1 && len(p.workerList) <= 1 {
		return fmt.Errorf("cannot remove the last bot from pool")
	}

	worker.disabled = true
	delete(p.workers, tokenHash)

	for i, w := range p.workerList {
		if w.tokenHash == tokenHash {
			p.workerList = append(p.workerList[:i], p.workerList[i+1:]...)
			break
		}
	}

	p.cond.Broadcast()
	return nil
}

// VerifyAccess checks that all active bots can post to the configured channel.
func (p *BotPool) VerifyAccess(ctx context.Context) error {
	p.mu.Lock()
	workers := make([]*botWorker, len(p.workerList))
	copy(workers, p.workerList)
	p.mu.Unlock()

	for _, worker := range workers {
		if worker.disabled {
			continue
		}
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			_, err = worker.bot.GetChat(ctx, &bot.GetChatParams{
				ChatID: p.channelID,
			})
			if err == nil {
				break
			}
			if attempt < 3 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt) * time.Second):
				}
			}
		}
		if err != nil {
			return fmt.Errorf("bot %s cannot access channel %d: %w", worker.tokenHash[:8], p.channelID, err)
		}
	}
	return nil
}

// AcquireWorker waits for and allocates an available worker for targetOwner whose capacity and cooldown allow a job.
// It returns the allocated worker and a release function that MUST be invoked when the transfer completes.
// Accepts optional ownerID (defaults to "admin").
func (p *BotPool) AcquireWorker(ctx context.Context, ownerID ...string) (*botWorker, func(), error) {
	targetOwner := "admin"
	if len(ownerID) > 0 && ownerID[0] != "" {
		targetOwner = ownerID[0]
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}

		activeEligibleCount := 0
		ownerWorkers := make([]*botWorker, 0)
		for _, w := range p.workerList {
			if w.ownerID == targetOwner {
				ownerWorkers = append(ownerWorkers, w)
				if !w.disabled && !w.downloadOnly {
					activeEligibleCount++
				}
			}
		}

		if activeEligibleCount == 0 {
			return nil, nil, fmt.Errorf("no active bot workers available in pool for owner %s", targetOwner)
		}

		effectiveOwnerLimit := p.maxUserConcurrency
		if effectiveOwnerLimit <= 0 || effectiveOwnerLimit > activeEligibleCount*p.maxPerBotConcurrency {
			effectiveOwnerLimit = activeEligibleCount * p.maxPerBotConcurrency
		}

		now := time.Now()
		var candidate *botWorker
		var earliestCooldown time.Time

		// Strictly allocate from owner's bots, subject to owner's concurrency limit and global concurrency
		if p.ownerActive[targetOwner] < effectiveOwnerLimit && p.globalActive < p.globalMaxConcurrency {
			curRR := p.roundRobin[targetOwner]
			for i := 0; i < len(ownerWorkers); i++ {
				idx := (curRR + i) % len(ownerWorkers)
				w := ownerWorkers[idx]
				if w.disabled || w.downloadOnly {
					continue
				}
				if now.Before(w.cooldownUntil) {
					if earliestCooldown.IsZero() || w.cooldownUntil.Before(earliestCooldown) {
						earliestCooldown = w.cooldownUntil
					}
					continue
				}
				if w.activeUploads < w.maxConcurrent {
					candidate = w
					p.roundRobin[targetOwner] = (idx + 1) % len(ownerWorkers)
					break
				}
			}
		}

		if candidate != nil {
			candidate.activeUploads++
			p.globalActive++
			p.ownerActive[targetOwner]++
			workerHash := candidate.tokenHash

			released := false
			release := func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				if released {
					return
				}
				released = true
				if w, ok := p.workers[workerHash]; ok {
					if w.activeUploads > 0 {
						w.activeUploads--
					}
				}
				if p.globalActive > 0 {
					p.globalActive--
				}
				if p.ownerActive[targetOwner] > 0 {
					p.ownerActive[targetOwner]--
				}
				p.cond.Broadcast()
			}
			return candidate, release, nil
		}

		waitDur := 250 * time.Millisecond
		if !earliestCooldown.IsZero() {
			rem := time.Until(earliestCooldown)
			if rem > 0 && rem < waitDur {
				waitDur = rem + 10*time.Millisecond
			}
		}

		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.mu.Lock()
			return nil, nil, ctx.Err()
		case <-time.After(waitDur):
			p.mu.Lock()
		}
	}
}

// UploadChunk sends an encrypted chunk as a document to the channel, automatically scheduling across eligible workers
// belonging to the specified owner. Accepts either (ctx, data, filename) [defaults to "admin"] or (ctx, ownerID, data, filename).
func (p *BotPool) UploadChunk(ctx context.Context, args ...interface{}) (*UploadResult, error) {
	var ownerID string = "admin"
	var data []byte
	var filename string

	if len(args) == 2 {
		var ok bool
		data, ok = args[0].([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid data argument")
		}
		filename, ok = args[1].(string)
		if !ok {
			return nil, fmt.Errorf("invalid filename argument")
		}
	} else if len(args) == 3 {
		var ok bool
		ownerID, ok = args[0].(string)
		if !ok || ownerID == "" {
			ownerID = "admin"
		}
		data, ok = args[1].([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid data argument")
		}
		filename, ok = args[2].(string)
		if !ok {
			return nil, fmt.Errorf("invalid filename argument")
		}
	} else {
		return nil, fmt.Errorf("invalid arguments to UploadChunk")
	}

	p.mu.Lock()
	numWorkers := 0
	for _, w := range p.workerList {
		if w.ownerID == ownerID && !w.disabled && !w.downloadOnly {
			numWorkers++
		}
	}
	p.mu.Unlock()

	if numWorkers == 0 {
		return nil, fmt.Errorf("no active bot workers available in pool for owner %s", ownerID)
	}

	maxAttempts := numWorkers * 2
	if maxAttempts < 3 {
		maxAttempts = 3
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		worker, release, err := p.AcquireWorker(ctx, ownerID)
		if err != nil {
			return nil, err
		}

		reader := bytes.NewReader(data)
		params := &bot.SendDocumentParams{
			ChatID: p.channelID,
			Document: &models.InputFileUpload{
				Filename: filename,
				Data:     reader,
			},
			DisableNotification: true,
		}

		// Perform network I/O outside of pool locks
		msg, err := worker.bot.SendDocument(ctx, params)
		release()

		if err == nil && msg != nil && msg.Document != nil {
			return &UploadResult{
				MessageID:    msg.ID,
				FileID:       msg.Document.FileID,
				BotTokenHash: worker.tokenHash,
			}, nil
		}

		// Don't penalize worker if caller cancelled
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return nil, ctx.Err()
		}

		p.classifyAndApplyError(worker, err, attempt)
	}

	return nil, fmt.Errorf("upload chunk failed after %d attempts: all available bots for owner %s are busy or down", maxAttempts, ownerID)
}

func (p *BotPool) classifyAndApplyError(w *botWorker, err error, attempt int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err == nil {
		return
	}

	errStr := err.Error()

	// 1. Rate limiting / Flood wait
	var tooMany *bot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		jitter := time.Duration(rand.Intn(500)) * time.Millisecond
		cooldown := time.Duration(tooMany.RetryAfter)*time.Second + jitter
		log.Printf("WARN: bot %s flood wait: %v (cooldown %v)", w.tokenHash[:8], err, cooldown)
		w.cooldownUntil = time.Now().Add(cooldown)
		p.cond.Broadcast()
		return
	}

	// 2. Authentication / token revoked / forbidden
	if errors.Is(err, bot.ErrorForbidden) || strings.Contains(strings.ToLower(errStr), "unauthorized") || strings.Contains(errStr, "401") || strings.Contains(errStr, "403") {
		log.Printf("ERROR: bot %s credentials revoked/disabled: %v", w.tokenHash[:8], err)
		w.disabled = true
		p.cond.Broadcast()
		return
	}

	// 3. Transient network errors / timeouts
	var netErr net.Error
	if errors.As(err, &netErr) || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "connection reset") {
		backoff := time.Duration(1<<uint(minInt(attempt, 2))) * time.Second
		jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
		cooldown := backoff + jitter
		log.Printf("WARN: bot %s transient network error: %v (cooldown %v)", w.tokenHash[:8], err, cooldown)
		w.cooldownUntil = time.Now().Add(cooldown)
		p.cond.Broadcast()
		return
	}

	// 4. Other general errors: short backoff
	backoff := 2*time.Second + time.Duration(rand.Intn(500))*time.Millisecond
	log.Printf("WARN: bot %s error: %v (cooldown %v)", w.tokenHash[:8], err, backoff)
	w.cooldownUntil = time.Now().Add(backoff)
	p.cond.Broadcast()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DownloadChunk fetches an encrypted chunk from Telegram by file_id using the EXACT bot that uploaded it.
func (p *BotPool) DownloadChunk(ctx context.Context, fileID, botTokenHash string) ([]byte, error) {
	p.mu.Lock()
	if alias, ok := p.hashAliases[botTokenHash]; ok {
		botTokenHash = alias
	}
	worker, ok := p.workers[botTokenHash]
	p.mu.Unlock()

	if !ok || botTokenHash == "" {
		// Fallback: try all bots in pool in case token was rotated, hash was legacy empty, or token hash changed
		data, err := p.DownloadFileAnyBot(ctx, fileID)
		if err == nil {
			return data, nil
		}
		return nil, fmt.Errorf("bot worker with hash %s not found in pool: telegram file_id is uniquely bound to the bot identity that uploaded it. Tried all pool bots fallback: %w", botTokenHash, err)
	}

	file, err := worker.bot.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "file is too big") {
			return nil, fmt.Errorf("telegram getFile failed: file is too big (>20MB Bot API download limit). Local Telegram Bot API server (TG_LOCAL_SERVER_URL) required or chunk must be re-uploaded with <=16MB chunk size: %w", err)
		}
		return nil, fmt.Errorf("get file info: %w", err)
	}

	if file.FilePath == "" {
		return nil, fmt.Errorf("file path empty for file_id %s", fileID)
	}

	baseURL := "https://api.telegram.org"
	if p.serverURL != "" {
		baseURL = strings.TrimRight(p.serverURL, "/")
	}
	downloadURL := fmt.Sprintf("%s/file/bot%s/%s", baseURL, worker.bot.Token(), file.FilePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	resp, err := worker.httpClient.Do(req)
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

// DownloadFileAnyBot fetches a file by fileID by trying all bots in the pool until one works.
func (p *BotPool) DownloadFileAnyBot(ctx context.Context, fileID string) ([]byte, error) {
	p.mu.Lock()
	workers := make([]*botWorker, len(p.workerList))
	copy(workers, p.workerList)
	p.mu.Unlock()

	baseURL := "https://api.telegram.org"
	if p.serverURL != "" {
		baseURL = strings.TrimRight(p.serverURL, "/")
	}

	for _, worker := range workers {
		if worker.disabled {
			continue
		}
		file, err := worker.bot.GetFile(ctx, &bot.GetFileParams{
			FileID: fileID,
		})
		if err != nil || file.FilePath == "" {
			continue
		}

		downloadURL := fmt.Sprintf("%s/file/bot%s/%s", baseURL, worker.bot.Token(), file.FilePath)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
		if err != nil {
			continue
		}

		resp, err := worker.httpClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		return data, nil
	}

	return nil, fmt.Errorf("file not found on any bot or download failed")
}

// DeleteMessage deletes a message from the channel using the first available active bot.
func (p *BotPool) DeleteMessage(ctx context.Context, messageID int) error {
	p.mu.Lock()
	var worker *botWorker
	for _, w := range p.workerList {
		if !w.disabled {
			worker = w
			break
		}
	}
	p.mu.Unlock()

	if worker == nil {
		return fmt.Errorf("no active bot worker available to delete message")
	}

	_, err := worker.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    p.channelID,
		MessageID: messageID,
	})
	return err
}

// GetStats returns the total and online bot count across the pool.
func (p *BotPool) GetStats() (total int, online int) {
	return p.GetStatsForOwner("")
}

// GetStatsForOwner returns the total and online bot count for a specific owner.
func (p *BotPool) GetStatsForOwner(ownerID string) (total int, online int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for _, w := range p.workerList {
		if ownerID == "" || ownerID == "admin" || w.ownerID == ownerID {
			total++
			if !w.disabled && now.After(w.cooldownUntil) {
				online++
			}
		}
	}
	return total, online
}

// GetBotStatuses returns detailed runtime status for each bot in the pool.
func (p *BotPool) GetBotStatuses() []BotStatus {
	return p.GetBotStatusesForOwner("")
}

// GetBotStatusesForOwner returns detailed runtime status for bots owned by ownerID.
func (p *BotPool) GetBotStatusesForOwner(ownerID string) []BotStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	statuses := make([]BotStatus, 0, len(p.workerList))
	for _, w := range p.workerList {
		if ownerID != "" && ownerID != "admin" && w.ownerID != ownerID {
			continue
		}
		isOnline := !w.disabled && now.After(w.cooldownUntil)
		var cd *time.Time
		if now.Before(w.cooldownUntil) {
			t := w.cooldownUntil
			cd = &t
		}
		statuses = append(statuses, BotStatus{
			TokenHash:     w.tokenHash,
			OwnerID:       w.ownerID,
			Online:        isOnline,
			Disabled:      w.disabled,
			ActiveUploads: w.activeUploads,
			CooldownUntil: cd,
		})
	}
	return statuses
}

// GetSafeCapacity calculates the safe number of concurrent uploads across the pool.
func (p *BotPool) GetSafeCapacity() int {
	return p.GetSafeCapacityForOwner("")
}

// GetSafeCapacityForOwner calculates safe concurrent uploads for ownerID based on eligible bot count.
func (p *BotPool) GetSafeCapacityForOwner(ownerID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	eligible := 0
	for _, w := range p.workerList {
		if (ownerID == "" || ownerID == "admin" || w.ownerID == ownerID) && !w.disabled && now.After(w.cooldownUntil) {
			eligible++
		}
	}

	if eligible == 0 {
		return 0
	}

	active := p.globalActive
	if ownerID != "" && ownerID != "admin" {
		active = p.ownerActive[ownerID]
	}

	available := eligible - active
	if available < 1 {
		available = 1
	}

	safeMax := 6
	if p.maxUserConcurrency > 0 {
		safeMax = p.maxUserConcurrency
	}
	if eligible < safeMax {
		safeMax = eligible
	}
	if available > safeMax {
		available = safeMax
	}
	return available
}

// DisableBot disables a bot from taking new uploads while keeping it in the worker map
// so existing chunks uploaded by this bot remain accessible.
// Supports either (tokenHash) defaulting to owner "admin", or (ownerID, tokenHash).
func (p *BotPool) DisableBot(args ...string) error {
	var ownerID, tokenHash string
	if len(args) == 1 {
		ownerID = "admin"
		tokenHash = args[0]
	} else if len(args) >= 2 {
		ownerID = args[0]
		tokenHash = args[1]
	} else {
		return fmt.Errorf("token hash required")
	}
	if ownerID == "" {
		ownerID = "admin"
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	worker, exists := p.workers[tokenHash]
	if !exists {
		return fmt.Errorf("bot worker with hash %s not found", tokenHash)
	}

	if worker.ownerID != ownerID && ownerID != "admin" {
		return fmt.Errorf("unauthorized: bot %s does not belong to owner %s", tokenHash, ownerID)
	}

	worker.disabled = true
	p.cond.Broadcast()
	return nil
}

// ExtractBotID returns the Telegram numeric bot user ID prefix from a bot token.
// Tokens are formatted as <bot_id>:<token_secret> (e.g. 123456789:ABCDef...).
func ExtractBotID(token string) string {
	idx := strings.Index(token, ":")
	if idx > 0 {
		return token[:idx]
	}
	return ""
}

// RotateBot rotates an existing bot worker's credentials.
// Distinguishes between rotating a token for the SAME Telegram bot identity vs REPLACING with a different bot identity:
// 1. Same Bot Identity: updates the worker client in place, preserves access to all existing file_ids,
//    and establishes hash alias so references to oldTokenHash resolve immediately to the new worker.
// 2. Different Bot Identity: registers the new bot as active for uploads, and preserves the old bot
//    as a download-only worker so existing chunks remain downloadable.
// Returns isSameBot bool, and error if initialization fails.
func (p *BotPool) RotateBot(ownerID, oldTokenHash, newToken string) (bool, error) {
	p.mu.Lock()
	oldWorker, exists := p.workers[oldTokenHash]
	if !exists {
		p.mu.Unlock()
		return false, fmt.Errorf("bot worker with hash %s not found", oldTokenHash)
	}
	if oldWorker.ownerID != ownerID && ownerID != "admin" {
		p.mu.Unlock()
		return false, fmt.Errorf("unauthorized: bot %s does not belong to owner %s", oldTokenHash, ownerID)
	}
	oldToken := oldWorker.token
	p.mu.Unlock()

	newHash := crypto.HashToken(newToken)
	oldBotID := ExtractBotID(oldToken)
	newBotID := ExtractBotID(newToken)
	isSameBot := (oldBotID != "" && oldBotID == newBotID)

	if isSameBot {
		httpClient := &http.Client{
			Transport: p.transport,
			Timeout:   10 * time.Minute,
		}
		opts := []bot.Option{
			bot.WithHTTPClient(10*time.Minute, httpClient),
			bot.WithSkipGetMe(),
		}
		if p.serverURL != "" {
			opts = append(opts, bot.WithServerURL(p.serverURL))
		}
		newB, err := bot.New(newToken, opts...)
		if err != nil {
			return false, fmt.Errorf("initialize rotated bot: %w", err)
		}

		p.mu.Lock()
		oldWorker.token = newToken
		oldWorker.tokenHash = newHash
		oldWorker.bot = newB
		oldWorker.httpClient = httpClient
		oldWorker.disabled = false
		oldWorker.downloadOnly = false

		if p.hashAliases == nil {
			p.hashAliases = make(map[string]string)
		}
		p.hashAliases[oldTokenHash] = newHash
		p.workers[newHash] = oldWorker
		p.workers[oldTokenHash] = oldWorker

		p.cond.Broadcast()
		p.mu.Unlock()

		return true, nil
	}

	// Different Bot Identity:
	if err := p.AddBot(ownerID, newToken); err != nil {
		return false, fmt.Errorf("register replacement bot: %w", err)
	}

	// Preserve old worker as download-only so its file_ids remain accessible
	p.mu.Lock()
	oldWorker.disabled = true
	oldWorker.downloadOnly = true
	p.cond.Broadcast()
	p.mu.Unlock()

	return false, nil
}

// UploadChunkWithBot sends an encrypted chunk directly through a specific bot worker.
func (p *BotPool) UploadChunkWithBot(ctx context.Context, botTokenHash string, data []byte, filename string) (*UploadResult, error) {
	p.mu.Lock()
	if alias, ok := p.hashAliases[botTokenHash]; ok {
		botTokenHash = alias
	}
	worker, exists := p.workers[botTokenHash]
	p.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("bot worker with hash %s not found", botTokenHash)
	}

	reader := bytes.NewReader(data)
	params := &bot.SendDocumentParams{
		ChatID: p.channelID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     reader,
		},
		DisableNotification: true,
	}

	msg, err := worker.bot.SendDocument(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("send document with bot %s: %w", worker.tokenHash[:8], err)
	}
	if msg == nil || msg.Document == nil {
		return nil, fmt.Errorf("telegram response missing document for bot %s", worker.tokenHash[:8])
	}

	return &UploadResult{
		MessageID:    msg.ID,
		FileID:       msg.Document.FileID,
		BotTokenHash: worker.tokenHash,
	}, nil
}

// DeleteChunkMessage deletes a Telegram message associated with a chunk using the bot that created it.
func (p *BotPool) DeleteChunkMessage(ctx context.Context, botTokenHash string, messageID int) error {
	p.mu.Lock()
	if alias, ok := p.hashAliases[botTokenHash]; ok {
		botTokenHash = alias
	}
	worker, exists := p.workers[botTokenHash]
	p.mu.Unlock()

	if !exists {
		return fmt.Errorf("bot worker with hash %s not found in pool", botTokenHash)
	}

	_, err := worker.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    p.channelID,
		MessageID: messageID,
	})
	return err
}

