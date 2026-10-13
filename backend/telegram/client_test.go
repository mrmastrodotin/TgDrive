package telegram

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

func TestBotPoolWorkerAcquisitionAndDynamicConcurrency(t *testing.T) {
	tokens := []string{"11111:AAABBBCCC", "22222:DDDEEEFFF", "33333:GGGHHHIII"}
	pool, err := NewBotPool(tokens, -100123456789, "")
	if err != nil {
		t.Fatalf("NewBotPool failed: %v", err)
	}

	ctx := context.Background()

	// 1. Initial pool stats
	total, online := pool.GetStats()
	if total != 3 || online != 3 {
		t.Fatalf("expected 3 total and 3 online, got %d total and %d online", total, online)
	}

	// 2. Acquire worker 1
	w1, rel1, err := pool.AcquireWorker(ctx)
	if err != nil {
		t.Fatalf("AcquireWorker 1 failed: %v", err)
	}
	if w1.activeUploads != 1 {
		t.Fatalf("expected w1 activeUploads 1, got %d", w1.activeUploads)
	}

	// 3. Acquire worker 2
	w2, rel2, err := pool.AcquireWorker(ctx)
	if err != nil {
		t.Fatalf("AcquireWorker 2 failed: %v", err)
	}
	if w2.tokenHash == w1.tokenHash {
		t.Fatalf("expected different worker, got same worker %s", w2.tokenHash)
	}

	// 4. Acquire worker 3
	w3, rel3, err := pool.AcquireWorker(ctx)
	if err != nil {
		t.Fatalf("AcquireWorker 3 failed: %v", err)
	}
	if w3.tokenHash == w1.tokenHash || w3.tokenHash == w2.tokenHash {
		t.Fatalf("expected third unique worker")
	}

	// 5. When all 3 bots are busy, acquiring with a short timeout should block and time out
	ctxTimeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, _, err = pool.AcquireWorker(ctxTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded when pool exhausted, got: %v", err)
	}

	// 6. Release worker 1 and verify another acquisition immediately succeeds
	rel1()
	w4, rel4, err := pool.AcquireWorker(ctx)
	if err != nil {
		t.Fatalf("expected immediate acquisition after release, got %v", err)
	}
	if w4.tokenHash != w1.tokenHash {
		t.Fatalf("expected acquired worker to be the released worker %s, got %s", w1.tokenHash, w4.tokenHash)
	}

	rel2()
	rel3()
	rel4()
}

func TestDynamicAddAndRemoveBot(t *testing.T) {
	tokens := []string{"11111:AAABBBCCC", "22222:DDDEEEFFF"}
	pool, err := NewBotPool(tokens, -100123456789, "")
	if err != nil {
		t.Fatalf("NewBotPool failed: %v", err)
	}

	// Add dynamic bot
	err = pool.AddBot("44444:JJJKKKLLL")
	if err != nil {
		t.Fatalf("AddBot failed: %v", err)
	}

	total, online := pool.GetStats()
	if total != 3 || online != 3 {
		t.Fatalf("expected 3 total bots after add, got %d total and %d online", total, online)
	}

	// Check status list
	statuses := pool.GetBotStatuses()
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}

	// Remove a bot
	removeHash := statuses[0].TokenHash
	err = pool.RemoveBot(removeHash)
	if err != nil {
		t.Fatalf("RemoveBot failed: %v", err)
	}

	total, _ = pool.GetStats()
	if total != 2 {
		t.Fatalf("expected 2 total bots after removal, got %d", total)
	}

	// Try removing unknown hash
	err = pool.RemoveBot("non-existent-hash")
	if err == nil {
		t.Fatalf("expected error removing non-existent bot")
	}
}

func TestErrorClassificationAndCooldown(t *testing.T) {
	tokens := []string{"11111:AAABBBCCC"}
	pool, err := NewBotPool(tokens, -100123456789, "")
	if err != nil {
		t.Fatalf("NewBotPool failed: %v", err)
	}

	worker := pool.workerList[0]

	// 1. Rate limiting error (TooManyRequestsError)
	rateLimitErr := &bot.TooManyRequestsError{
		Message:    "Too Many Requests: retry after 2",
		RetryAfter: 2,
	}
	pool.classifyAndApplyError(worker, rateLimitErr, 0)
	if worker.cooldownUntil.Before(time.Now().Add(1 * time.Second)) {
		t.Fatalf("expected cooldown of at least 1s for rate limit, got until %v", worker.cooldownUntil)
	}

	// Reset cooldown
	worker.cooldownUntil = time.Time{}

	// 2. Authentication error / 401 unauthorized
	authErr := errors.New("401 Unauthorized: bot token is invalid")
	pool.classifyAndApplyError(worker, authErr, 0)
	if !worker.disabled {
		t.Fatalf("expected worker to be marked disabled after 401 error")
	}

	// 3. Transient network error
	worker.disabled = false
	netErr := &net.DNSError{IsTimeout: true}
	pool.classifyAndApplyError(worker, netErr, 1)
	if worker.cooldownUntil.Before(time.Now()) {
		t.Fatalf("expected cooldown after transient net error")
	}
}

func TestContextCancellationDoesNotPenalizeBot(t *testing.T) {
	tokens := []string{"11111:AAABBBCCC"}
	pool, err := NewBotPool(tokens, -100123456789, "")
	if err != nil {
		t.Fatalf("NewBotPool failed: %v", err)
	}

	worker := pool.workerList[0]
	initialCooldown := worker.cooldownUntil

	// Caller cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Calling UploadChunk with cancelled ctx should immediately return context.Canceled without penalizing bot
	_, err = pool.UploadChunk(ctx, []byte("data"), "test.chunk.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	if worker.cooldownUntil != initialCooldown {
		t.Fatalf("worker cooldown was changed despite caller cancellation: %v", worker.cooldownUntil)
	}
}

func TestTwoUserBotPoolIsolation(t *testing.T) {
	pool, err := NewBotPool(nil, -100123456789, "")
	if err != nil {
		t.Fatalf("NewBotPool failed: %v", err)
	}

	// Register 2 bots for Alice and 1 bot for Bob
	aliceToken1 := "11111:ALICE_BOT_ALPHA"
	aliceToken2 := "22222:ALICE_BOT_BETA"
	bobToken1 := "33333:BOB_BOT_PRIMARY"

	if err := pool.AddBot("alice", aliceToken1); err != nil {
		t.Fatalf("AddBot alice 1 failed: %v", err)
	}
	if err := pool.AddBot("alice", aliceToken2); err != nil {
		t.Fatalf("AddBot alice 2 failed: %v", err)
	}
	if err := pool.AddBot("bob", bobToken1); err != nil {
		t.Fatalf("AddBot bob 1 failed: %v", err)
	}

	ctx := context.Background()

	// 1. Verify stats isolation
	totA, onlA := pool.GetStatsForOwner("alice")
	if totA != 2 || onlA != 2 {
		t.Fatalf("expected Alice to have 2 total / 2 online bots, got %d / %d", totA, onlA)
	}
	totB, onlB := pool.GetStatsForOwner("bob")
	if totB != 1 || onlB != 1 {
		t.Fatalf("expected Bob to have 1 total / 1 online bot, got %d / %d", totB, onlB)
	}
	totC, onlC := pool.GetStatsForOwner("charlie")
	if totC != 0 || onlC != 0 {
		t.Fatalf("expected Charlie to have 0 bots, got %d / %d", totC, onlC)
	}

	if capA := pool.GetSafeCapacityForOwner("alice"); capA != 2 {
		t.Fatalf("expected Alice capacity 2, got %d", capA)
	}
	if capB := pool.GetSafeCapacityForOwner("bob"); capB != 1 {
		t.Fatalf("expected Bob capacity 1, got %d", capB)
	}
	if capC := pool.GetSafeCapacityForOwner("charlie"); capC != 0 {
		t.Fatalf("expected Charlie capacity 0, got %d", capC)
	}

	// 2. Verify status list isolation
	statListA := pool.GetBotStatusesForOwner("alice")
	if len(statListA) != 2 {
		t.Fatalf("expected Alice status list len 2, got %d", len(statListA))
	}
	for _, st := range statListA {
		if st.OwnerID != "alice" {
			t.Fatalf("expected bot in Alice's list to belong to alice, got %s", st.OwnerID)
		}
	}

	statListB := pool.GetBotStatusesForOwner("bob")
	if len(statListB) != 1 {
		t.Fatalf("expected Bob status list len 1, got %d", len(statListB))
	}
	if statListB[0].OwnerID != "bob" {
		t.Fatalf("expected bot in Bob's list to belong to bob, got %s", statListB[0].OwnerID)
	}

	// 3. Worker acquisition isolation:
	// Bob acquires his 1 worker. Bob's worker is now busy.
	wB1, relB1, err := pool.AcquireWorker(ctx, "bob")
	if err != nil {
		t.Fatalf("Bob AcquireWorker failed: %v", err)
	}
	if wB1.ownerID != "bob" {
		t.Fatalf("expected Bob's worker to have owner bob, got %s", wB1.ownerID)
	}

	// Bob attempts to acquire a second worker.
	// Even though Alice's 2 bots are completely idle, Bob MUST NOT borrow Alice's bots.
	ctxTimeout, cancel := context.WithTimeout(ctx, 60*time.Millisecond)
	_, _, err = pool.AcquireWorker(ctxTimeout, "bob")
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded when Bob's pool is exhausted, got: %v", err)
	}

	// Alice concurrently acquires both of her workers without any interference from Bob
	wA1, relA1, err := pool.AcquireWorker(ctx, "alice")
	if err != nil {
		t.Fatalf("Alice AcquireWorker 1 failed: %v", err)
	}
	if wA1.ownerID != "alice" {
		t.Fatalf("expected Alice's worker to have owner alice, got %s", wA1.ownerID)
	}

	wA2, relA2, err := pool.AcquireWorker(ctx, "alice")
	if err != nil {
		t.Fatalf("Alice AcquireWorker 2 failed: %v", err)
	}
	if wA2.ownerID != "alice" {
		t.Fatalf("expected Alice's worker to have owner alice, got %s", wA2.ownerID)
	}

	// Release workers
	relB1()
	relA1()
	relA2()

	// 4. User without registered bots fails immediately
	_, _, err = pool.AcquireWorker(ctx, "charlie")
	if err == nil || !strings.Contains(err.Error(), "no active bot workers available") {
		t.Fatalf("expected error for user with 0 bots, got: %v", err)
	}

	// 5. Cross-user management isolation:
	aliceHash := statListA[0].TokenHash

	// Bob cannot remove Alice's bot
	if err := pool.RemoveBot("bob", aliceHash); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error when Bob tries to remove Alice's bot, got: %v", err)
	}

	// Bob cannot disable Alice's bot
	if err := pool.DisableBot("bob", aliceHash); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error when Bob tries to disable Alice's bot, got: %v", err)
	}

	// Bob cannot rotate Alice's bot
	if _, err := pool.RotateBot("bob", aliceHash, "44444:NEW_TOKEN"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error when Bob tries to rotate Alice's bot, got: %v", err)
	}

	// Bob cannot steal Alice's bot token
	if err := pool.AddBot("bob", aliceToken1); err == nil || !strings.Contains(err.Error(), "already owned by another user") {
		t.Fatalf("expected ownership conflict when Bob tries to register Alice's token, got: %v", err)
	}

	// 6. Disabling owner's own bot takes effect
	bobHash := statListB[0].TokenHash
	if err := pool.DisableBot("bob", bobHash); err != nil {
		t.Fatalf("Bob DisableBot failed: %v", err)
	}

	if capB := pool.GetSafeCapacityForOwner("bob"); capB != 0 {
		t.Fatalf("expected Bob capacity 0 after disabling his only bot, got %d", capB)
	}

	_, _, err = pool.AcquireWorker(ctx, "bob")
	if err == nil || !strings.Contains(err.Error(), "no active bot workers available") {
		t.Fatalf("expected acquisition failure for Bob after disabling his bot, got: %v", err)
	}
}

func TestRotateBotSameBotVsDifferentBot(t *testing.T) {
	pool, err := NewBotPool(nil, -100123456789, "")
	if err != nil {
		t.Fatalf("NewBotPool failed: %v", err)
	}

	// Register bot with ID 12345
	origToken := "12345:ORIGINAL_SECRET"
	if err := pool.AddBot("alice", origToken); err != nil {
		t.Fatalf("AddBot failed: %v", err)
	}
	origHash := pool.GetBotStatusesForOwner("alice")[0].TokenHash

	// 1. Same-Bot Rotation: token for same bot ID 12345
	rotatedToken := "12345:ROTATED_SECRET"
	isSameBot, err := pool.RotateBot("alice", origHash, rotatedToken)
	if err != nil {
		t.Fatalf("RotateBot (same bot) failed: %v", err)
	}
	if !isSameBot {
		t.Fatalf("expected isSameBot = true for matching bot ID 12345")
	}

	// Check that alias was created and original worker remains active
	pool.mu.Lock()
	aliasTarget, hasAlias := pool.hashAliases[origHash]
	pool.mu.Unlock()
	if !hasAlias || aliasTarget == "" {
		t.Fatalf("expected hashAliases to map %s to new hash", origHash)
	}

	// Worker should still be acquired for uploads
	ctx := context.Background()
	w, rel, err := pool.AcquireWorker(ctx, "alice")
	if err != nil {
		t.Fatalf("AcquireWorker failed after same-bot rotation: %v", err)
	}
	if w.token != rotatedToken {
		t.Fatalf("expected acquired worker to have rotated token, got %s", w.token)
	}
	rel()

	// 2. Different-Bot Replacement: new bot with ID 99999
	newBotToken := "99999:COMPLETELY_NEW_BOT"
	rotatedHash := pool.GetBotStatusesForOwner("alice")[0].TokenHash
	isSameBot2, err := pool.RotateBot("alice", rotatedHash, newBotToken)
	if err != nil {
		t.Fatalf("RotateBot (different bot) failed: %v", err)
	}
	if isSameBot2 {
		t.Fatalf("expected isSameBot = false for different bot ID 99999")
	}

	// Verify that the old bot (12345) is retained as downloadOnly
	pool.mu.Lock()
	oldWorker := pool.workers[rotatedHash]
	pool.mu.Unlock()
	if oldWorker == nil {
		t.Fatalf("expected old bot worker to be retained in pool")
	}
	if !oldWorker.downloadOnly {
		t.Fatalf("expected old bot worker to be marked downloadOnly")
	}

	// New worker (99999) should be active and acquired for new uploads
	wNew, relNew, err := pool.AcquireWorker(ctx, "alice")
	if err != nil {
		t.Fatalf("AcquireWorker failed after different-bot replacement: %v", err)
	}
	if wNew.token != newBotToken {
		t.Fatalf("expected acquired worker to be the new bot, got %s", wNew.token)
	}
	relNew()
}


