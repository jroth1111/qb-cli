package profiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectAPITypeREST(t *testing.T) {
	path := writeTempSpec(t, "spec.json", `{"openapi": "3.0.0"}`)

	if got := DetectAPIType(path); got != APITypeREST {
		t.Fatalf("DetectAPIType(%q) = %q, want %q", path, got, APITypeREST)
	}
}

func TestDetectAPITypeGraphQL(t *testing.T) {
	path := writeTempSpec(t, "schema.graphql", `type Query { users: [User] }`)

	if got := DetectAPIType(path); got != APITypeGraphQL {
		t.Fatalf("DetectAPIType(%q) = %q, want %q", path, got, APITypeGraphQL)
	}
}

func TestDetectAPITypeGRPC(t *testing.T) {
	path := writeTempSpec(t, "service.proto", `syntax = "proto3";`)

	if got := DetectAPIType(path); got != APITypeGRPC {
		t.Fatalf("DetectAPIType(%q) = %q, want %q", path, got, APITypeGRPC)
	}
}

func TestDetectAPITypeURL(t *testing.T) {
	path := "https://api.example.com/graphql"

	if got := DetectAPIType(path); got != APITypeGraphQL {
		t.Fatalf("DetectAPIType(%q) = %q, want %q", path, got, APITypeGraphQL)
	}
}

func TestDetectAPITypeYAMLFallback(t *testing.T) {
	path := writeTempSpec(t, "spec.yaml", "title: Example API\nversion: v1\n")

	if got := DetectAPIType(path); got != APITypeREST {
		t.Fatalf("DetectAPIType(%q) = %q, want %q", path, got, APITypeREST)
	}
}

func writeTempSpec(t *testing.T, name, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp spec: %v", err)
	}

	return path
}
