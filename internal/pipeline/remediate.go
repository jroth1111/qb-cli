package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// RemediationResult summarizes what auto-remediation fixed (or reverted).
type RemediationResult struct {
	FlagsRemoved    []string `json:"flags_removed"`
	FuncsRemoved    []string `json:"funcs_removed"`
	TablesRemoved   []string `json:"tables_removed"`
	FTSRemoved      []string `json:"fts_removed"`
	CompileAfterFix bool     `json:"compile_after_fix"`
	Reverted        bool     `json:"reverted"`
}

type fileBackup struct {
	path    string
	content []byte
}

func backupFiles(paths []string) ([]fileBackup, error) {
	var backups []fileBackup
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("backing up %s: %w", p, err)
		}
		backups = append(backups, fileBackup{path: p, content: data})
	}
	return backups, nil
}

func restoreBackups(backups []fileBackup) error {
	for _, b := range backups {
		if err := os.WriteFile(b.path, b.content, 0o644); err != nil {
			return fmt.Errorf("restoring %s: %w", b.path, err)
		}
	}
	return nil
}

// Remediate applies safe auto-remediation (deletions only) based on a verification report.
// If the compile gate fails after fixes, all changes are reverted.
func Remediate(dir string, report *VerificationReport) (*RemediationResult, error) {
	if report == nil {
		return &RemediationResult{}, nil
	}

	var deadFlags []string
	for _, f := range report.Flags {
		if f.DeadFlag || f.References == 0 {
			deadFlags = append(deadFlags, f.FlagName)
		}
	}

	var ghostTables []string
	var orphanFTS []string
	for _, p := range report.Pipeline {
		if p.GhostTable || !p.HasWrite {
			ghostTables = append(ghostTables, p.TableName)
		}
		if p.OrphanFTS || (p.HasFTS && !p.HasSearch) {
			orphanFTS = append(orphanFTS, p.TableName+"_fts")
		}
	}

	if len(deadFlags) == 0 && len(ghostTables) == 0 && len(orphanFTS) == 0 {
		return &RemediationResult{}, nil
	}

	filesToBackup := []string{
		filepath.Join(dir, "internal", "cli", "root.go"),
		filepath.Join(dir, "internal", "cli", "helpers.go"),
		filepath.Join(dir, "internal", "store", "store.go"),
	}

	backups, err := backupFiles(filesToBackup)
	if err != nil {
		return nil, fmt.Errorf("creating backups: %w", err)
	}

	result := &RemediationResult{}

	if len(deadFlags) > 0 {
		if err := removeDeadFlags(dir, deadFlags); err != nil {
			_ = restoreBackups(backups)
			return nil, fmt.Errorf("removing dead flags: %w", err)
		}
		result.FlagsRemoved = deadFlags
	}

	if len(ghostTables) > 0 {
		if err := removeGhostTables(dir, ghostTables); err != nil {
			_ = restoreBackups(backups)
			return nil, fmt.Errorf("removing ghost tables: %w", err)
		}
		result.TablesRemoved = ghostTables
	}

	if len(orphanFTS) > 0 {
		if err := removeOrphanFTS(dir, orphanFTS); err != nil {
			_ = restoreBackups(backups)
			return nil, fmt.Errorf("removing orphan FTS: %w", err)
		}
		result.FTSRemoved = orphanFTS
	}

	if err := compileGateCheck(dir); err != nil {
		_ = restoreBackups(backups)
		result.CompileAfterFix = false
		result.Reverted = true
		return result, fmt.Errorf("compile gate failed after remediation, changes reverted: %w", err)
	}

	result.CompileAfterFix = true
	return result, nil
}

func removeDeadFlags(dir string, deadFlags []string) error {
	rootPath := filepath.Join(dir, "internal", "cli", "root.go")
	data, err := os.ReadFile(rootPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading root.go: %w", err)
	}

	content := string(data)

	for _, flagName := range deadFlags {
		regLine := regexp.MustCompile(`(?m)^[^\n]*&flags\.` + regexp.QuoteMeta(flagName) + `\b[^\n]*\n`)
		content = regLine.ReplaceAllString(content, "")

		structField := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(flagName) + `\s+\S+[^\n]*\n`)
		content = structField.ReplaceAllString(content, "")
	}

	return os.WriteFile(rootPath, []byte(content), 0o644)
}

func removeGhostTables(dir string, ghostTables []string) error {
	storePath := filepath.Join(dir, "internal", "store", "store.go")
	data, err := os.ReadFile(storePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading store.go: %w", err)
	}

	content := string(data)

	for _, tableName := range ghostTables {
		createRe := regexp.MustCompile(`(?is)CREATE TABLE\s+(?:IF NOT EXISTS\s+)?` + regexp.QuoteMeta(tableName) + `\s*\(.*?\);`)
		content = createRe.ReplaceAllString(content, "")

		indexRe := regexp.MustCompile(`(?im)^[^\n]*CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF NOT EXISTS\s+)?\w+\s+ON\s+` + regexp.QuoteMeta(tableName) + `\b[^;]*;[^\n]*\n?`)
		content = indexRe.ReplaceAllString(content, "")
	}

	return os.WriteFile(storePath, []byte(content), 0o644)
}

func removeOrphanFTS(dir string, orphanTables []string) error {
	storePath := filepath.Join(dir, "internal", "store", "store.go")
	data, err := os.ReadFile(storePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading store.go: %w", err)
	}

	content := string(data)

	for _, ftsName := range orphanTables {
		virtualRe := regexp.MustCompile(`(?is)CREATE VIRTUAL TABLE\s+(?:IF NOT EXISTS\s+)?` + regexp.QuoteMeta(ftsName) + `\s+USING\s+fts5\s*\(.*?\);`)
		content = virtualRe.ReplaceAllString(content, "")

		baseName := strings.TrimSuffix(ftsName, "_fts")

		triggerRe := regexp.MustCompile(`(?is)CREATE TRIGGER\s+(?:IF NOT EXISTS\s+)?\w+\s+AFTER\s+(?:INSERT|DELETE|UPDATE)\s+ON\s+` + regexp.QuoteMeta(baseName) + `\b.*?END\s*;`)
		content = triggerRe.ReplaceAllString(content, "")
	}

	return os.WriteFile(storePath, []byte(content), 0o644)
}

func compileGateCheck(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	build := exec.CommandContext(ctx, "go", "build", "./...")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build failed: %w\n%s", err, out)
	}

	vet := exec.CommandContext(ctx, "go", "vet", "./...")
	vet.Dir = dir
	if out, err := vet.CombinedOutput(); err != nil {
		return fmt.Errorf("go vet failed: %w\n%s", err, out)
	}

	return nil
}
