package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

const (
	expectedModulePath      = "github.com/uptrace/terraform"
	expectedProviderAddress = "registry.terraform.io/uptrace/uptrace"
	expectedReleaseProject  = "terraform-provider-uptrace"
)

// TestReleaseIdentityMatchesRegistryProviderName pins terraform:G1.
// It holds terraform:F1: the registry address and the release artifact names.
func TestReleaseIdentityMatchesRegistryProviderName(t *testing.T) {
	t.Helper()

	goMod := readReleaseFile(t, "go.mod")
	requireLine(t, goMod, `module\s+`+regexp.QuoteMeta(expectedModulePath), "go.mod module path")

	mainGo := readReleaseFile(t, "main.go")
	if !strings.Contains(mainGo, `Address: "`+expectedProviderAddress+`"`) {
		t.Fatalf("main.go provider address must be %q", expectedProviderAddress)
	}

	taskfile := readReleaseFile(t, "Taskfile.yml")
	requireLine(t, taskfile, `\s*PKG_NAME:\s*`+regexp.QuoteMeta(expectedModulePath), "Taskfile package path")
	requireLine(t, taskfile, `\s*BINARY:\s*`+regexp.QuoteMeta(expectedReleaseProject), "Taskfile binary name")

	goreleaser := readReleaseFile(t, ".goreleaser.yml")
	requireLine(t, goreleaser, `project_name:\s*`+regexp.QuoteMeta(expectedReleaseProject), "GoReleaser project name")
	if !strings.Contains(goreleaser, `-X `+expectedModulePath+`/version.ProviderVersion={{.Version}}`) {
		t.Fatalf("GoReleaser ldflags must set ProviderVersion through %s/version", expectedModulePath)
	}
	requireLine(t, goreleaser, `\s*binary:\s*"\{\{ \.ProjectName \}\}_v\{\{ \.Version \}\}"`, "GoReleaser binary template")
	requireLine(t, goreleaser, `\s*-\s*formats:\s*\["zip"\]`, "GoReleaser archive formats")
	requireLine(t, goreleaser, `\s*name_template:\s*"\{\{ \.ProjectName \}\}_\{\{ \.Version \}\}_\{\{ \.Os \}\}_\{\{ \.Arch \}\}"`, "GoReleaser archive template")
	requireLine(t, goreleaser, `\s*name_template:\s*"\{\{ \.ProjectName \}\}_\{\{ \.Version \}\}_SHA256SUMS"`, "GoReleaser checksum template")

	if regexp.MustCompile(`(?m)^\s*(-\s*)?format:\s*`).MatchString(goreleaser) {
		t.Fatal("GoReleaser archive config must use formats instead of deprecated format")
	}
}

func TestTerraformRegistryManifestPublishesFrameworkProtocol(t *testing.T) {
	t.Helper()

	var manifest struct {
		Version  int `json:"version"`
		Metadata struct {
			ProtocolVersions []string `json:"protocol_versions"`
		} `json:"metadata"`
	}
	data := []byte(readReleaseFile(t, "terraform-registry-manifest.json"))
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("terraform-registry-manifest.json must be valid JSON: %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
	if got := manifest.Metadata.ProtocolVersions; len(got) != 1 || got[0] != "6.0" {
		t.Fatalf("manifest protocol_versions = %#v, want []string{\"6.0\"}", got)
	}

	goreleaser := readReleaseFile(t, ".goreleaser.yml")
	requireManifestExtraFile(t, goreleaser, "checksum")
	requireManifestExtraFile(t, goreleaser, "release")
}

func readReleaseFile(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func requireManifestExtraFile(t *testing.T, goreleaser, section string) {
	t.Helper()

	body := requireTopLevelSection(t, goreleaser, section)
	requireLine(t, body, `\s*extra_files:`, section+" manifest extra_files")
	requireLine(t, body, `\s*-\s*glob:\s*['"]?terraform-registry-manifest\.json['"]?`, section+" manifest glob")
	requireLine(t, body, `\s*name_template:\s*['"]?\{\{ \.ProjectName \}\}_\{\{ \.Version \}\}_manifest\.json['"]?`, section+" manifest release name")
}

func requireTopLevelSection(t *testing.T, body, section string) string {
	t.Helper()

	startRe := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(section) + `:\s*$`)
	loc := startRe.FindStringIndex(body)
	if loc == nil {
		t.Fatalf("missing top-level %s section", section)
	}

	rest := body[loc[1]:]
	endRe := regexp.MustCompile(`(?m)^[A-Za-z_]+:\s*$`)
	if end := endRe.FindStringIndex(rest); end != nil {
		return rest[:end[0]]
	}
	return rest
}

func requireLine(t *testing.T, body, pattern, description string) {
	t.Helper()

	re := regexp.MustCompile(`(?m)^` + pattern + `$`)
	if !re.MatchString(body) {
		t.Fatalf("%s must match %q", description, pattern)
	}
}
