package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkspacePathsUseScopedPressHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PRINTING_PRESS_HOME", home)
	t.Setenv("PRINTING_PRESS_SCOPE", "atlanta-v1-deadbeef")

	assert.Equal(t, filepath.Join(home, ".runstate"), RunstateRoot())
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef"), ScopedRunstateRoot())
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "current"), CurrentRunDir())
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "current", "notion.json"), CurrentRunPointerPath("notion"))
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "runs", "run-123"), RunRoot("run-123"))
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "runs", "run-123", "working", "notion-pp-cli"), WorkingCLIDir("notion", "run-123"))
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "runs", "run-123", "research"), RunResearchDir("run-123"))
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "runs", "run-123", "proofs"), RunProofsDir("run-123"))
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "runs", "run-123", "pipeline"), RunPipelineDir("run-123"))
	assert.Equal(t, filepath.Join(home, ".runstate", "atlanta-v1-deadbeef", "runs", "run-123", "discovery"), RunDiscoveryDir("run-123"))
	assert.Equal(t, filepath.Join(home, "library"), PublishedLibraryRoot())
	assert.Equal(t, filepath.Join(home, "manuscripts", "notion", "run-123"), ArchivedManuscriptDir("notion", "run-123"))
	assert.Equal(t, filepath.Join(home, "manuscripts", "notion", "run-123", "discovery"), ArchivedDiscoveryDir("notion", "run-123"))
	assert.Equal(t, filepath.Join(home, "workspaces", "atlanta-v1-deadbeef", "manuscripts", "notion", "research"), ResearchDir("notion"))
	assert.Equal(t, filepath.Join(home, "workspaces", "atlanta-v1-deadbeef", "manuscripts", "notion", "proofs"), ProofsDir("notion"))
}

func TestWorkspaceScopeSanitizesRepoBaseName(t *testing.T) {
	t.Setenv("PRINTING_PRESS_SCOPE", "")
	t.Setenv("PRINTING_PRESS_REPO_ROOT", "/tmp/Atlanta V1!!")

	assert.Contains(t, WorkspaceScope(), "atlanta-v1")
}
