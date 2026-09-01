package tests

import (
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/services"
)

func TestWriteBucketSpendsAndRefills(t *testing.T) {
	bucket := services.NewTokenBucket(3, 300*time.Millisecond)

	for i := 1; i <= 3; i++ {
		if !bucket.Allow("ana") {
			t.Fatalf("write %d should have been allowed", i)
		}
	}
	if bucket.Allow("ana") {
		t.Fatal("the fourth write should have been refused")
	}

	// Budgets are per identity: one person running out must not affect another.
	if !bucket.Allow("bruno") {
		t.Fatal("a different identity must have its own budget")
	}

	time.Sleep(350 * time.Millisecond)
	if !bucket.Allow("ana") {
		t.Fatal("the bucket should have refilled")
	}
}

func TestWriteBucketRefusesWithoutIdentity(t *testing.T) {
	if services.NewTokenBucket(10, time.Minute).Allow("") {
		t.Fatal("an empty identity must never be granted a token")
	}
}

func TestPINGuardLocksTheLinkNotTheVisitor(t *testing.T) {
	guard := services.NewPINGuard(3, 200*time.Millisecond)

	if locked, _ := guard.Locked("link-a"); locked {
		t.Fatal("a fresh link must not start locked")
	}

	for i := 0; i < 3; i++ {
		guard.Failed("link-a")
	}

	locked, remaining := guard.Locked("link-a")
	if !locked {
		t.Fatal("the link should be locked once the budget is spent")
	}
	if remaining <= 0 {
		t.Fatal("a locked link should report how long is left")
	}

	// The lock belongs to the link under attack, not to whoever knocked.
	if otherLocked, _ := guard.Locked("link-b"); otherLocked {
		t.Fatal("locking one link must not lock another")
	}

	time.Sleep(250 * time.Millisecond)
	if stillLocked, _ := guard.Locked("link-a"); stillLocked {
		t.Fatal("the lock should expire on its own")
	}
}

func TestPINGuardForgetsAfterSuccess(t *testing.T) {
	guard := services.NewPINGuard(2, time.Minute)
	guard.Failed("link-c")
	guard.Succeeded("link-c")
	guard.Failed("link-c")

	if locked, _ := guard.Locked("link-c"); locked {
		t.Fatal("a correct PIN should clear the record, not carry failures over")
	}
}

func TestPrivateDestinationsAreRefusedByDefault(t *testing.T) {
	for _, raw := range []string{
		"http://10.0.0.5/wiki",
		"http://192.168.1.1/admin",
		"http://169.254.169.254/latest/meta-data/",
	} {
		if _, _, err := services.CleanURL(raw, "link.kaicorplabs.com"); err == nil {
			t.Errorf("%s should be refused unless the operator opts in", raw)
		}
	}
}
