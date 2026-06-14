package instance

import (
	"strings"
	"testing"
)

func TestSlugDisambiguatesSharedBaseNames(t *testing.T) {
	// The collision that motivated slugs: two repos whose base dir is the same
	// name. They must NOT share a routing identifier.
	a := NewInstance("/home/johnm/src/github.com/jmylchreest", "jmylchreest", "/a.sock", "/a.db", "1.0.0")
	b := NewInstance("/home/johnm/src/github.com/jmylchreest/jmylchreest", "jmylchreest", "/b.sock", "/b.db", "1.0.0")

	if a.Slug() == b.Slug() {
		t.Fatalf("colliding base names produced identical slugs: %q", a.Slug())
	}
	// The readable name stays the prefix (display value is unchanged).
	if !strings.HasPrefix(a.Slug(), "jmylchreest-") {
		t.Errorf("Slug() = %q, want prefix %q", a.Slug(), "jmylchreest-")
	}
	// Deterministic: same root → same slug (stable across restarts/bookmarks).
	if got := Slug("jmylchreest", "/home/johnm/src/github.com/jmylchreest"); got != a.Slug() {
		t.Errorf("Slug not deterministic: %q vs %q", got, a.Slug())
	}
	// The hash suffix is the configured length.
	if suffix := a.Slug()[len("jmylchreest-"):]; len(suffix) != slugHashLen {
		t.Errorf("hash suffix %q len = %d, want %d", suffix, len(suffix), slugHashLen)
	}
}

func TestNewInstance(t *testing.T) {
	inst := NewInstance("/home/user/project", "my-project", "/tmp/aide.sock", "/home/user/project/.aide/memory/memory.db", "1.0.0")

	if got := inst.ProjectRoot(); got != "/home/user/project" {
		t.Errorf("ProjectRoot() = %q, want %q", got, "/home/user/project")
	}
	if got := inst.ProjectName(); got != "my-project" {
		t.Errorf("ProjectName() = %q, want %q", got, "my-project")
	}
	if got := inst.SocketPath(); got != "/tmp/aide.sock" {
		t.Errorf("SocketPath() = %q, want %q", got, "/tmp/aide.sock")
	}
	if got := inst.DBPath(); got != "/home/user/project/.aide/memory/memory.db" {
		t.Errorf("DBPath() = %q, want %q", got, "/home/user/project/.aide/memory/memory.db")
	}
	if got := inst.Version(); got != "1.0.0" {
		t.Errorf("Version() = %q, want %q", got, "1.0.0")
	}
	if got := inst.Status(); got != StatusDisconnected {
		t.Errorf("Status() = %q, want %q", got, StatusDisconnected)
	}
	if got := inst.LastSeen(); !got.IsZero() {
		t.Errorf("LastSeen() = %v, want zero", got)
	}
	if got := inst.Client(); got != nil {
		t.Errorf("Client() = %v, want nil", got)
	}
	if got := inst.Store(); got != nil {
		t.Errorf("Store() = %v, want nil", got)
	}
	if got := inst.FindingsStore(); got != nil {
		t.Errorf("FindingsStore() = %v, want nil", got)
	}
	if got := inst.SurveyStore(); got != nil {
		t.Errorf("SurveyStore() = %v, want nil", got)
	}
}

func TestUpdateMeta(t *testing.T) {
	inst := NewInstance("/root", "proj", "/old.sock", "/old.db", "0.1.0")

	inst.UpdateMeta("/new.sock", "/new.db", "2.0.0")

	if got := inst.SocketPath(); got != "/new.sock" {
		t.Errorf("SocketPath() after UpdateMeta = %q, want %q", got, "/new.sock")
	}
	if got := inst.DBPath(); got != "/new.db" {
		t.Errorf("DBPath() after UpdateMeta = %q, want %q", got, "/new.db")
	}
	if got := inst.Version(); got != "2.0.0" {
		t.Errorf("Version() after UpdateMeta = %q, want %q", got, "2.0.0")
	}
	// ProjectRoot and ProjectName should be unchanged
	if got := inst.ProjectRoot(); got != "/root" {
		t.Errorf("ProjectRoot() changed unexpectedly to %q", got)
	}
	if got := inst.ProjectName(); got != "proj" {
		t.Errorf("ProjectName() changed unexpectedly to %q", got)
	}
}

func TestDisconnectIdempotent(t *testing.T) {
	inst := NewInstance("/root", "proj", "/sock", "/db", "1.0.0")

	// Disconnect on a fresh (already disconnected) instance should not panic
	inst.Disconnect()
	inst.Disconnect()

	if got := inst.Status(); got != StatusDisconnected {
		t.Errorf("Status() after double Disconnect = %q, want %q", got, StatusDisconnected)
	}
}

func TestHealthCheckWithoutConnection(t *testing.T) {
	inst := NewInstance("/root", "proj", "/sock", "/db", "1.0.0")

	// HealthCheck without a connection should return false
	if inst.HealthCheck() {
		t.Error("HealthCheck() = true on disconnected instance, want false")
	}
}
