package worklog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"ai-bricklaying/internal/safeio"
)

var ErrFlowBusy = errors.New("daily flow is busy")
var ErrInvalidFlowState = errors.New("invalid daily flow state")
var ErrArtifactConflict = errors.New("confirmed worklog artifact already exists")

type Store struct {
	// writeFile is a test seam for exercising multi-file rollback. Production
	// callers leave it nil and use safeio.WriteFile.
	writeFile func(string, []byte, safeio.WriteOptions) error
}

// Acquire serializes read-check-write mutations across CLI processes. Atomic
// file replacement alone cannot provide compare-and-swap semantics.
func (Store) Acquire(configDir, date string) (func(), error) {
	lockPath := (Store{}).StatePath(configDir, date) + ".lock"
	return acquireStoreLock(lockPath)
}

// AcquireArtifacts serializes finalization for output paths shared by flows
// that may use different config roots.
func (Store) AcquireArtifacts(outputDir, date string) (func(), error) {
	lockPath := filepath.Join(outputDir, ".ai-bricklaying", "locks", "v1", "daily", date+".lock")
	return acquireStoreLock(lockPath)
}

func acquireStoreLock(lockPath string) (func(), error) {
	if err := safeio.RejectSymlinkAncestors(lockPath); err != nil {
		return nil, err
	}
	if err := safeio.EnsureDir(filepath.Dir(lockPath), safeio.PrivateDirMode); err != nil {
		return nil, err
	}
	if err := safeio.RejectSymlinkAncestors(lockPath); err != nil {
		return nil, err
	}
	return acquireProcessLock(lockPath)
}

func (Store) StatePath(configDir, date string) string {
	return filepath.Join(configDir, "state", "v1", "daily", date+".json")
}

func (Store) WorklogPaths(outputDir, date string) (string, string) {
	base := filepath.Join(outputDir, "worklogs", "daily", date+"-ai-bricklaying-worklog")
	return base + ".md", base + ".json"
}

func (Store) Load(configDir, date string) (DailyFlow, error) {
	path := (Store{}).StatePath(configDir, date)
	if err := safeio.RejectSymlinkAncestors(path); err != nil {
		return DailyFlow{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return DailyFlow{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return DailyFlow{}, fmt.Errorf("%w: %s", safeio.ErrSymlinkTarget, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return DailyFlow{}, err
	}
	var flow DailyFlow
	if err := json.Unmarshal(contents, &flow); err != nil {
		return DailyFlow{}, fmt.Errorf("%w: malformed JSON: %v", ErrInvalidFlowState, err)
	}
	if flow.SchemaVersion != SchemaVersion {
		return DailyFlow{}, fmt.Errorf("%w: unsupported schema %q", ErrInvalidFlowState, flow.SchemaVersion)
	}
	if err := validateFlow(flow, configDir, date); err != nil {
		return DailyFlow{}, err
	}
	return flow, nil
}

func (Store) Exists(configDir, date string) (bool, error) {
	path := (Store{}).StatePath(configDir, date)
	if err := safeio.RejectSymlinkAncestors(path); err != nil {
		return false, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return false, fmt.Errorf("%w: %s", safeio.ErrSymlinkTarget, path)
	}
	return true, nil
}

func (store Store) Save(flow DailyFlow) error {
	if err := validateFlow(flow, flow.ConfigDir, flow.Date); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(flow, "", "  ")
	if err != nil {
		return err
	}
	return store.write(flow.StatePath, append(contents, '\n'), safeio.WriteOptions{
		FileMode: safeio.PrivateFileMode,
		DirMode:  safeio.PrivateDirMode,
	})
}

// CommitConfirmed writes both public artifacts and confirmed state as one
// failure-consistent operation. State is written last; if any write fails, all
// targets already changed by this call are restored to their prior snapshots.
func (store Store) CommitConfirmed(flow DailyFlow, confirmed ConfirmedWorklog, markdown string) error {
	_, err := store.CommitConfirmedFlow(flow, confirmed, markdown)
	return err
}

// CommitConfirmedFlow also returns the exact state persisted by a recovered
// interrupted commit. If a prior JSON artifact exists, its confirmation time is
// retained so the artifact and resumed state remain identical.
func (store Store) CommitConfirmedFlow(flow DailyFlow, confirmed ConfirmedWorklog, markdown string) (DailyFlow, error) {
	if err := validateFlow(flow, flow.ConfigDir, flow.Date); err != nil {
		return DailyFlow{}, err
	}
	existing, exists, err := recoverableConfirmedJSONFile(flow.WorklogJSONPath, confirmed)
	if err != nil {
		return DailyFlow{}, err
	}
	if exists {
		recoveredAt := existing.ConfirmedAt
		flow.ConfirmedAt = &recoveredAt
		flow.UpdatedAt = recoveredAt
		for index := len(flow.Idempotency) - 1; index >= 0; index-- {
			if flow.Idempotency[index].Command == "daily.finalize" && flow.Idempotency[index].Revision == flow.Revision {
				flow.Idempotency[index].CreatedAt = recoveredAt
				break
			}
		}
		confirmed.ConfirmedAt = recoveredAt
		if err := validateFlow(flow, flow.ConfigDir, flow.Date); err != nil {
			return DailyFlow{}, err
		}
	}
	stateContents, err := json.MarshalIndent(flow, "", "  ")
	if err != nil {
		return DailyFlow{}, err
	}
	jsonContents, err := json.MarshalIndent(confirmed, "", "  ")
	if err != nil {
		return DailyFlow{}, err
	}
	jsonContents = append(jsonContents, '\n')
	markdownContents := []byte(safeio.RedactString(markdown))
	targets := []transactionTarget{
		{path: flow.WorklogJSONPath, contents: jsonContents, artifact: true},
		{path: flow.WorklogMarkdownPath, contents: markdownContents, artifact: true},
		{path: flow.StatePath, contents: append(stateContents, '\n')},
	}
	seen := map[string]bool{}
	for index := range targets {
		clean := filepath.Clean(targets[index].path)
		if seen[clean] {
			return DailyFlow{}, fmt.Errorf("%w: duplicate persistence target", ErrInvalidFlowState)
		}
		seen[clean] = true
		if err := safeio.RejectSymlinkAncestors(clean); err != nil {
			return DailyFlow{}, err
		}
		snapshot, err := snapshotFile(clean)
		if err != nil {
			return DailyFlow{}, err
		}
		if targets[index].artifact && snapshot.exists {
			if !bytes.Equal(snapshot.contents, targets[index].contents) {
				return DailyFlow{}, fmt.Errorf("%w: %s", ErrArtifactConflict, clean)
			}
			targets[index].skip = true
		}
		targets[index].path = clean
		targets[index].before = snapshot
	}

	changed := make([]transactionTarget, 0, len(targets))
	for _, target := range targets {
		if target.skip {
			continue
		}
		options := safeio.WriteOptions{FileMode: safeio.PrivateFileMode, DirMode: safeio.PrivateDirMode, NoClobber: target.artifact}
		if err := store.write(target.path, target.contents, options); err != nil {
			if errors.Is(err, safeio.ErrTargetExists) {
				if rollbackErr := rollbackTargets(changed); rollbackErr != nil {
					return DailyFlow{}, fmt.Errorf("%w: %s (rollback failed: %v)", ErrArtifactConflict, target.path, rollbackErr)
				}
				return DailyFlow{}, fmt.Errorf("%w: %s", ErrArtifactConflict, target.path)
			}
			attempted := append(changed, target)
			if rollbackErr := rollbackTargets(attempted); rollbackErr != nil {
				return DailyFlow{}, fmt.Errorf("confirmed worklog commit failed: %w (rollback failed: %v)", err, rollbackErr)
			}
			return DailyFlow{}, err
		}
		changed = append(changed, target)
	}
	return flow, nil
}

func recoverableConfirmedJSON(contents []byte, expected ConfirmedWorklog) (ConfirmedWorklog, bool) {
	var existing ConfirmedWorklog
	if err := json.Unmarshal(contents, &existing); err != nil || existing.ConfirmedAt.IsZero() {
		return ConfirmedWorklog{}, false
	}
	expected.ConfirmedAt = existing.ConfirmedAt
	canonical, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		return ConfirmedWorklog{}, false
	}
	if !bytes.Equal(contents, append(canonical, '\n')) {
		return ConfirmedWorklog{}, false
	}
	return existing, true
}

func recoverableConfirmedJSONFile(path string, expected ConfirmedWorklog) (ConfirmedWorklog, bool, error) {
	if err := safeio.RejectSymlinkAncestors(path); err != nil {
		return ConfirmedWorklog{}, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ConfirmedWorklog{}, false, nil
	}
	if err != nil {
		return ConfirmedWorklog{}, false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return ConfirmedWorklog{}, false, fmt.Errorf("%w: %s", safeio.ErrSymlinkTarget, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ConfirmedWorklog{}, false, err
	}
	existing, ok := recoverableConfirmedJSON(contents, expected)
	if !ok {
		return ConfirmedWorklog{}, false, fmt.Errorf("%w: %s", ErrArtifactConflict, path)
	}
	return existing, true, nil
}

// EnsureArtifactsAvailable prevents two config roots from claiming the same
// date-keyed output artifact. It must run while the output artifact lock is held.
func (Store) EnsureArtifactsAvailable(flow DailyFlow) error {
	for _, path := range []string{flow.WorklogJSONPath, flow.WorklogMarkdownPath} {
		if err := safeio.RejectSymlinkAncestors(path); err != nil {
			return err
		}
		_, err := os.Lstat(path)
		if err == nil {
			return fmt.Errorf("%w: %s", ErrArtifactConflict, path)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (store Store) write(path string, contents []byte, options safeio.WriteOptions) error {
	if store.writeFile != nil {
		return store.writeFile(path, contents, options)
	}
	return safeio.WriteFile(path, contents, options)
}

type fileSnapshot struct {
	exists   bool
	contents []byte
	mode     fs.FileMode
}

type transactionTarget struct {
	path     string
	contents []byte
	before   fileSnapshot
	artifact bool
	skip     bool
}

func snapshotFile(path string) (fileSnapshot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fileSnapshot{}, fmt.Errorf("%w: %s", safeio.ErrSymlinkTarget, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{exists: true, contents: contents, mode: info.Mode().Perm()}, nil
}

func rollbackTargets(targets []transactionTarget) error {
	var rollbackErrors []error
	for index := len(targets) - 1; index >= 0; index-- {
		target := targets[index]
		if target.before.exists {
			mode := target.before.mode
			if mode == 0 {
				mode = safeio.PrivateFileMode
			}
			if err := safeio.WriteFile(target.path, target.before.contents, safeio.WriteOptions{FileMode: mode, DirMode: safeio.PrivateDirMode}); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
			continue
		}
		if err := safeio.RejectSymlinkAncestors(target.path); err != nil {
			rollbackErrors = append(rollbackErrors, err)
			continue
		}
		if err := os.Remove(target.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	return errors.Join(rollbackErrors...)
}

func validateFlow(flow DailyFlow, configDir, date string) error {
	configDir = filepath.Clean(configDir)
	if !filepath.IsAbs(configDir) || flow.ConfigDir != configDir || flow.Date != date {
		return fmt.Errorf("%w: config root or date mismatch", ErrInvalidFlowState)
	}
	expectedState := (Store{}).StatePath(configDir, date)
	if !filepath.IsAbs(flow.StatePath) || flow.StatePath != expectedState {
		return fmt.Errorf("%w: state path is not anchored to config root", ErrInvalidFlowState)
	}
	if !filepath.IsAbs(flow.OutputDir) || filepath.Clean(flow.OutputDir) != flow.OutputDir {
		return fmt.Errorf("%w: output root must be an absolute clean path", ErrInvalidFlowState)
	}
	expectedMarkdown, expectedJSON := (Store{}).WorklogPaths(flow.OutputDir, date)
	if flow.WorklogMarkdownPath != expectedMarkdown || flow.WorklogJSONPath != expectedJSON {
		return fmt.Errorf("%w: worklog paths are not anchored to output root", ErrInvalidFlowState)
	}
	if flow.State != StateDraft && flow.State != StateInterviewing && flow.State != StateConfirmed {
		return fmt.Errorf("%w: unsupported lifecycle state", ErrInvalidFlowState)
	}
	sanitizedInterview, err := sanitizeInterview(flow.Interview)
	if err != nil || sanitizedInterview.Stage != flow.Interview.Stage || sanitizedInterview.NextQuestion != flow.Interview.NextQuestion || !equalStrings(sanitizedInterview.CompletedQuestions, flow.Interview.CompletedQuestions) {
		return fmt.Errorf("%w: invalid interview progress", ErrInvalidFlowState)
	}
	if flow.Consent.RemoteEvidence != "pending" && flow.Consent.RemoteEvidence != "granted" && flow.Consent.RemoteEvidence != "denied" {
		return fmt.Errorf("%w: unsupported consent state", ErrInvalidFlowState)
	}
	if len(flow.Evidence) > 0 && flow.Consent.RemoteEvidence == "pending" && flow.State != StateDraft {
		return fmt.Errorf("%w: evidence-backed progress requires an explicit consent decision", ErrInvalidFlowState)
	}
	if flow.State == StateConfirmed && flow.ConfirmedAt == nil {
		return fmt.Errorf("%w: confirmed state requires confirmed_at", ErrInvalidFlowState)
	}
	if flow.State == StateConfirmed && flow.Interview.Stage != InterviewComplete {
		return fmt.Errorf("%w: confirmed state requires complete interview progress", ErrInvalidFlowState)
	}
	if flow.State == StateDraft && !isEmptyInterview(flow.Interview) {
		return fmt.Errorf("%w: draft state cannot contain interview progress", ErrInvalidFlowState)
	}
	if flow.State == StateInterviewing && !isEmptyInterview(flow.Interview) {
		if err := validateCheckpointInterview(flow.Interview); err != nil {
			return fmt.Errorf("%w: invalid interview progress", ErrInvalidFlowState)
		}
	}
	if flow.State == StateConfirmed && (flow.Interview.NextQuestion != "" || !equalStrings(flow.Interview.CompletedQuestions, previewStepIDs)) {
		return fmt.Errorf("%w: confirmed state requires every interview question to be complete", ErrInvalidFlowState)
	}
	sanitizedWorkItems, err := sanitizeWorkItems(flow.WorkItems, flow.Evidence)
	if err != nil || !equalWorkItems(flow.WorkItems, sanitizedWorkItems) {
		return fmt.Errorf("%w: invalid work items", ErrInvalidFlowState)
	}
	if sanitizedReflection := sanitizeReflection(flow.Reflection); flow.Reflection != sanitizedReflection {
		return fmt.Errorf("%w: invalid reflection", ErrInvalidFlowState)
	}
	confirmedCount := 0
	for _, item := range flow.WorkItems {
		if item.Status == WorkItemConfirmed {
			confirmedCount++
		}
	}
	if flow.NoWorkConfirmed && confirmedCount > 0 {
		return fmt.Errorf("%w: no-work decision conflicts with confirmed work", ErrInvalidFlowState)
	}
	if flow.State == StateConfirmed && confirmedCount == 0 && !flow.NoWorkConfirmed {
		return fmt.Errorf("%w: confirmed empty flow requires no-work decision", ErrInvalidFlowState)
	}
	return nil
}

func equalWorkItems(left, right []WorkItem) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftItem, rightItem := left[index], right[index]
		if leftItem.ID != rightItem.ID || leftItem.Title != rightItem.Title ||
			leftItem.EvidenceSummary != rightItem.EvidenceSummary || leftItem.Uncertainty != rightItem.Uncertainty ||
			leftItem.Performed != rightItem.Performed || leftItem.Outcome != rightItem.Outcome ||
			leftItem.Verification != rightItem.Verification || leftItem.Issues != rightItem.Issues ||
			leftItem.Status != rightItem.Status || leftItem.Origin != rightItem.Origin ||
			!equalStrings(leftItem.EvidenceIDs, rightItem.EvidenceIDs) {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
