package tests

import (
	"os"
	"testing"

	"github.com/Ulzuhan/linkup/internal/config"
)

// Administration is decided by the provider's group when one is configured, and
// only by the username list when none is. The two never combine: if they did,
// removing someone from the group would not actually remove them.

func TestAdminComesFromTheGroupWhenOneIsConfigured(t *testing.T) {
	t.Setenv("LINKUP_ADMIN_GROUP", "linkup-admins")
	t.Setenv("LINKUP_ADMIN_USERS", "ana")
	t.Setenv("LINKUP_HOST", "127.0.0.1")
	cfg := config.Load()

	if !cfg.IsAdmin("bruno", []string{"linkup", "linkup-admins"}) {
		t.Error("someone in the admin group must be an administrator")
	}
	if cfg.IsAdmin("bruno", []string{"linkup"}) {
		t.Error("someone outside the group must not be an administrator")
	}
	// The username list is ignored while a group governs, on purpose.
	if cfg.IsAdmin("ana", []string{"linkup"}) {
		t.Error("the username fallback must not override the group")
	}
	// An API key carries no groups, so it never administers.
	if cfg.IsAdmin("ana", nil) {
		t.Error("no groups means no administration when a group is configured")
	}
}

func TestAdminFallsBackToTheUserListWithoutAGroup(t *testing.T) {
	os.Unsetenv("LINKUP_ADMIN_GROUP")
	t.Setenv("LINKUP_ADMIN_USERS", "ana, bruno")
	t.Setenv("LINKUP_HOST", "127.0.0.1")
	cfg := config.Load()

	if !cfg.IsAdmin("ana", nil) {
		t.Error("without a group, the username list decides")
	}
	if cfg.IsAdmin("carla", nil) {
		t.Error("someone outside the list must not be an administrator")
	}
}
