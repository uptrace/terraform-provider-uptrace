package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The provider lives in a subdirectory of the uptrace/app checkout, so the
// root that LoadEnv reads .env from must be this module, not the git root.
func TestFindModuleRootIsTheProviderModule(t *testing.T) {
	root, ok := findModuleRoot()
	if !ok {
		t.Fatal("findModuleRoot found no root")
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if first, _, _ := strings.Cut(string(data), "\n"); first != "module github.com/uptrace/terraform" {
		t.Fatalf("root %s holds %q, want the provider module", root, first)
	}
}
