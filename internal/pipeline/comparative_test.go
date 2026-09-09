package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScoreAlternative(t *testing.T) {
	t.Run("binary install gets 20", func(t *testing.T) {
		alt := Alternative{Name: "go-cli", InstallMethod: "binary"}
		scored := scoreAlternative(alt, 10)
		assert.Equal(t, 20, scored.InstallFriction)
	})

	t.Run("npm install gets 10", func(t *testing.T) {
		alt := Alternative{Name: "node-cli", InstallMethod: "npm"}
		scored := scoreAlternative(alt, 10)
		assert.Equal(t, 10, scored.InstallFriction)
	})

	t.Run("unknown install gets 5", func(t *testing.T) {
		alt := Alternative{Name: "weird-cli", InstallMethod: "something-else"}
		scored := scoreAlternative(alt, 10)
		assert.Equal(t, 5, scored.InstallFriction)
	})
}

func TestCompareGapsAndAdvantages(t *testing.T) {
	result := &ComparativeResult{
		OurScore: 95,
		Alternatives: []AltScore{
			{Name: "some-tool", Breadth: 10, Total: 40},
		},
	}
	gaps, advantages := compareGapsAndAdvantages(result)
	assert.NotEmpty(t, advantages)

	foundGoBinary := false
	for _, a := range advantages {
		if a == "Go binary - zero runtime dependencies, instant startup" {
			foundGoBinary = true
		}
	}
	assert.True(t, foundGoBinary, "should always include Go binary advantage")
	assert.NotEmpty(t, gaps)
}

func TestRunComparative(t *testing.T) {
	dir := t.TempDir()
	// No research.json exists - should still produce a result without panicking
	result, err := RunComparative(dir, 10)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 95, result.OurScore)
	assert.Equal(t, "ship", result.Recommendation)
}

func TestRunComparativeLoadsResearchFromSiblingResearchDir(t *testing.T) {
	runRoot := t.TempDir()
	pipelineDir := filepath.Join(runRoot, "pipeline")
	researchDir := filepath.Join(runRoot, "research")

	require.NoError(t, os.MkdirAll(researchDir, 0o755))
	research := &ResearchResult{
		Alternatives: []Alternative{
			{Name: "competitor/sample-cli", InstallMethod: "binary", CommandCount: 12},
		},
	}
	data, err := json.MarshalIndent(research, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(researchDir, "research.json"), data, 0o644))

	result, err := RunComparative(pipelineDir, 10)
	require.NoError(t, err)
	assert.Len(t, result.Alternatives, 1)
}
