package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOpenAPI_ValidOpenAPI3JSON(t *testing.T) {
	data := []byte(`{"openapi":"3.0.0","info":{"title":"Test"},"paths":{}}`)
	assert.True(t, IsOpenAPI(data))
}

func TestIsOpenAPI_ValidSwagger2JSON(t *testing.T) {
	data := []byte(`{"swagger":"2.0","info":{"title":"Test"},"paths":{}}`)
	assert.True(t, IsOpenAPI(data))
}

func TestIsOpenAPI_ValidOpenAPI3YAML(t *testing.T) {
	data := []byte("openapi: \"3.0.0\"\ninfo:\n  title: Test\npaths: {}")
	assert.True(t, IsOpenAPI(data))
}

func TestIsOpenAPI_InternalYAML(t *testing.T) {
	data := []byte("name: test\nresources:\n  users:\n    endpoints: {}")
	assert.False(t, IsOpenAPI(data))
}

func TestIsOpenAPI_RandomJSON(t *testing.T) {
	data := []byte(`{"foo":"bar"}`)
	assert.False(t, IsOpenAPI(data))
}

func TestIsOpenAPI_EmptyInput(t *testing.T) {
	assert.False(t, IsOpenAPI([]byte{}))
}

func TestIsOpenAPI_BinaryGarbage(t *testing.T) {
	data := []byte{0xFF, 0xFE, 0x00, 0x01}
	assert.False(t, IsOpenAPI(data))
}
