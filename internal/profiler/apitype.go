package profiler

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

type APIType string

const (
	APITypeREST    APIType = "rest"
	APITypeGraphQL APIType = "graphql"
	APITypeGRPC    APIType = "grpc"
	APITypeUnknown APIType = "unknown"
)

// DetectAPIType determines the API type from a spec file path or URL.
func DetectAPIType(specPath string) APIType {
	lower := strings.ToLower(specPath)
	ext := strings.ToLower(filepath.Ext(specPath))

	// URL-based detection
	if strings.Contains(lower, "/graphql") || strings.HasSuffix(lower, "/graphql") {
		return APITypeGraphQL
	}

	// Extension-based detection
	switch ext {
	case ".graphql", ".gql":
		return APITypeGraphQL
	case ".proto":
		return APITypeGRPC
	}

	// Content-based detection (read first 1000 bytes)
	if data, err := readHead(specPath, 1000); err == nil {
		content := string(data)
		if strings.Contains(content, "openapi") || strings.Contains(content, "swagger") {
			return APITypeREST
		}
		if strings.Contains(content, "type Query") || strings.Contains(content, "type Mutation") || strings.Contains(content, "schema {") {
			return APITypeGraphQL
		}
		if strings.Contains(content, `syntax = "proto`) {
			return APITypeGRPC
		}
	}

	// YAML/JSON specs are typically REST
	if ext == ".yaml" || ext == ".yml" || ext == ".json" {
		return APITypeREST
	}

	return APITypeUnknown
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, n)
	nr, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:nr], nil
}
