package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckText_CatchesAISlop(t *testing.T) {
	t.Run("comprehensive solution triggers warning", func(t *testing.T) {
		warnings := CheckText("This is a comprehensive solution for your needs.")
		assert.NotEmpty(t, warnings)
		found := false
		for _, w := range warnings {
			if w.Match == "comprehensive" {
				found = true
			}
		}
		assert.True(t, found)
	})

	t.Run("robust framework triggers warning", func(t *testing.T) {
		warnings := CheckText("A robust framework for building CLIs.")
		assert.NotEmpty(t, warnings)
		found := false
		for _, w := range warnings {
			if w.Match == "robust" {
				found = true
			}
		}
		assert.True(t, found)
	})
}

func TestCheckText_NormalText(t *testing.T) {
	t.Run("the API returns JSON has zero warnings", func(t *testing.T) {
		warnings := CheckText("the API returns JSON")
		assert.Empty(t, warnings)
	})

	t.Run("run the command has zero warnings", func(t *testing.T) {
		warnings := CheckText("run the command")
		assert.Empty(t, warnings)
	})
}

func TestFormatWarnings_Empty(t *testing.T) {
	result := FormatWarnings(nil)
	assert.Equal(t, "", result)
}

func TestFormatWarnings_WithMatches(t *testing.T) {
	warnings := []AITextWarning{
		{Pattern: "p1", Match: "comprehensive", Line: 1, Context: "a comprehensive guide"},
		{Pattern: "p2", Match: "robust", Line: 3, Context: "robust solution"},
		{Pattern: "p3", Match: "seamless", Line: 5, Context: "seamless integration"},
	}
	result := FormatWarnings(warnings)
	assert.Contains(t, result, "3 warning(s)")
	assert.Contains(t, result, "comprehensive")
	assert.Contains(t, result, "robust")
	assert.Contains(t, result, "seamless")
}
