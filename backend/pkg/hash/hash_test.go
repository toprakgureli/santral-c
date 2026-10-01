package hash

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	encoded, err := Password("correct horse battery")
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	if !Compare(encoded, "correct horse battery") {
		t.Fatal("Compare rejected the correct password")
	}
	if Compare(encoded, "wrong password") {
		t.Fatal("Compare accepted a wrong password")
	}
}

func TestCompareRejectsMalformedHash(t *testing.T) {
	for _, bad := range []string{"", "plain", "$argon2id$bad", "$bcrypt$v=19$m=1,t=1,p=1$a$b"} {
		if Compare(bad, "x") {
			t.Fatalf("Compare accepted malformed hash %q", bad)
		}
	}
}

func TestPasswordProducesDistinctHashes(t *testing.T) {
	a, _ := Password("same")
	b, _ := Password("same")
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestTokenAndSHA256(t *testing.T) {
	tok, err := Token(32)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if len(tok) == 0 {
		t.Fatal("Token returned empty string")
	}
	if SHA256("x") == SHA256("y") {
		t.Fatal("SHA256 collided on different inputs")
	}
	if len(SHA256("x")) != 64 {
		t.Fatal("SHA256 must be 64 hex chars")
	}
}

// TestHashingIsBounded: fifty sign-ins at once never run more than
// hashSlots password checks together, so memory stays bounded.
func TestHashingIsBounded(t *testing.T) {
	encoded, err := Password("office password")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	running, most := 0, 0
	orig := argonKey
	argonKey = func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
		mu.Lock()
		running++
		most = max(most, running)
		mu.Unlock()
		defer func() {
			mu.Lock()
			running--
			mu.Unlock()
		}()
		return orig(password, salt, time, memory, threads, keyLen)
	}
	t.Cleanup(func() { argonKey = orig })

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !Compare(encoded, "office password") {
				t.Error("Compare rejected the correct password")
			}
		}()
	}
	wg.Wait()
	if most > hashSlots {
		t.Errorf("%d password checks ran at once, want at most %d", most, hashSlots)
	}
	if most < 2 {
		t.Errorf("checks ran one by one (%d at most); the slots are not used", most)
	}
}

// TestCheckGivesUpWhenBusy: with every slot taken, a sign-in gets ErrBusy
// instead of waiting for ever.
func TestCheckGivesUpWhenBusy(t *testing.T) {
	encoded, err := Password("office password")
	if err != nil {
		t.Fatal(err)
	}
	for range hashSlots {
		slots <- struct{}{}
	}
	defer func() {
		for range hashSlots {
			<-slots
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Check(ctx, encoded, "office password"); !errors.Is(err, ErrBusy) {
		t.Fatalf("Check with every slot busy returned %v, want ErrBusy", err)
	}
}

func TestCheck(t *testing.T) {
	encoded, err := Password("office password")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := Check(context.Background(), encoded, "office password"); !ok || err != nil {
		t.Fatalf("Check rejected the right password: %v %v", ok, err)
	}
	if ok, err := Check(context.Background(), encoded, "wrong"); ok || err != nil {
		t.Fatalf("Check accepted a wrong password: %v %v", ok, err)
	}
}
