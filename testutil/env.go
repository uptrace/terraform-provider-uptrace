// Package testutil provides shared helpers for tests across the provider.
package testutil

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadEnv reads a .env file from the module root and sets any KEY=VALUE
// entries in the process environment. Shell exports always win.
//
// The module root is the nearest directory, from the current working
// directory up, that holds a go.mod file. It is not the git root: in the
// uptrace/app checkout the git root is the app, whose .env is not for the
// provider.
func LoadEnv() {
	root, ok := findModuleRoot()
	if !ok {
		return
	}
	_ = godotenv.Load(filepath.Join(root, ".env"))
}

func findModuleRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
