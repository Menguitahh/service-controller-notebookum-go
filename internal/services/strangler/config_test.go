package strangler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "strangler.yaml")
	if err := os.WriteFile(path, []byte(`
rules:
  - route_pattern: "POST /api/v1/users"
    destination: "service-user"
    enabled: true
  - route_pattern: "GET /api/v1/users/:id"
    destination: "service-user"
    enabled: true
`), 0o600); err != nil {
		t.Fatal(err)
	}

	rule, err := FindRule("GET", "/api/v1/users/123", path)
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil || rule.Destination != "service-user" {
		t.Fatal("expected matching rule")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("C:\\does-not-exist.yaml"); err == nil {
		t.Fatal("expected error")
	}
}
