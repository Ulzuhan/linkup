package services

import (
	"context"
	"testing"

	"github.com/Ulzuhan/linkup/internal/models"
)

// Includes a real HTTP UserInfo request and SQLite lookup on every iteration.
// No production IdP, database or session is used; setup is outside the timer.
func BenchmarkLiveOIDCSession(b *testing.B) {
	f := newLiveAuth(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.auth.GetSession(f.request()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLiveOIDCAPIKey(b *testing.B) {
	f := newLiveAuth(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := &models.UserSession{Username: "alice"}
		if err := f.auth.AuthorizeAPIKey(context.Background(), user); err != nil {
			b.Fatal(err)
		}
	}
}
