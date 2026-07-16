package safeio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestWriteFileRefusesSymlinkTargetAndPreservesLinkedContent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	link := filepath.Join(root, "out", "summary.md")
	if err := os.WriteFile(target, []byte("original target content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := WriteFile(link, []byte("replacement"), WriteOptions{})
	if !errors.Is(err, ErrSymlinkTarget) {
		t.Fatalf("error = %v, want ErrSymlinkTarget", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "original target content" {
		t.Fatalf("target content = %q, want unchanged", contents)
	}
}

func TestWriteFileRefusesExistingSymlinkAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	linkedDir := filepath.Join(root, "linked")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatal(err)
	}

	err := WriteFile(filepath.Join(linkedDir, "nested", "artifact.json"), []byte("redirected"), WriteOptions{})
	if !errors.Is(err, ErrSymlinkTarget) {
		t.Fatalf("error = %v, want ErrSymlinkTarget", err)
	}
	if _, err := os.Stat(filepath.Join(realDir, "nested", "artifact.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("redirected target should not be created: %v", err)
	}
}

func TestWriteFilePreservesExistingContentWhenTempCreateFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions are not authoritative on Windows")
	}

	root := t.TempDir()
	path := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(path, []byte("still here"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	err := WriteFile(path, []byte("replacement"), WriteOptions{DirMode: 0o500})
	if err == nil {
		t.Fatal("expected temp-file creation failure")
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "still here" {
		t.Fatalf("existing content = %q, want preserved", contents)
	}
}

func TestWriteFileCreatesDirectoryAndWritesAtomically(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "artifact.json")

	if err := WriteFile(path, []byte(`{"ok":true}`), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "ok") {
		t.Fatalf("contents = %q, want written payload", contents)
	}
}

func TestWriteFileReplacesExistingFileByDefault(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	if err := os.WriteFile(path, []byte("old state"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(path, []byte("new state"), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new state" {
		t.Fatalf("state contents = %q, want replacement", contents)
	}
}

func TestWriteFileNoClobberAtomicallyAllowsOneConcurrentCreator(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifact.json")
	const writers = 32

	start := make(chan struct{})
	results := make(chan error, writers)
	var ready sync.WaitGroup
	ready.Add(writers)
	for index := 0; index < writers; index++ {
		index := index
		go func() {
			ready.Done()
			<-start
			contents := []byte(fmt.Sprintf(`{"writer":%d}`, index))
			results <- WriteFile(path, contents, WriteOptions{NoClobber: true})
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	conflicts := 0
	for index := 0; index < writers; index++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrTargetExists):
			conflicts++
		default:
			t.Fatalf("unexpected writer error: %v", err)
		}
	}
	if successes != 1 || conflicts != writers-1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 and %d", successes, conflicts, writers-1)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(contents), `{"writer":`) {
		t.Fatalf("artifact contents = %q, want one complete writer payload", contents)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("directory entries = %v, want only committed artifact", entryNames(entries))
	}
}

func TestWriteFileNoClobberPreservesExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifact.json")
	if err := os.WriteFile(path, []byte("external contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteFile(path, []byte("replacement"), WriteOptions{NoClobber: true})
	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("error = %v, want ErrTargetExists", err)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "external contents" {
		t.Fatalf("artifact contents = %q, want external contents preserved", contents)
	}
}

func TestWriteFileNoClobberPreservesPrivateMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not authoritative on Windows")
	}

	root := t.TempDir()
	path := filepath.Join(root, "private", "artifact.json")
	options := WriteOptions{FileMode: PrivateFileMode, DirMode: PrivateDirMode, NoClobber: true}
	if err := WriteFile(path, []byte("private contents"), options); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != PrivateFileMode {
		t.Fatalf("artifact mode = %#o, want %#o", got, PrivateFileMode)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != PrivateDirMode {
		t.Fatalf("artifact directory mode = %#o, want %#o", got, PrivateDirMode)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestWriteFileSupportsPlatformTempDirectoryPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "artifact.json")
	if err := WriteFile(path, []byte(`{"ok":true}`), WriteOptions{}); err != nil {
		t.Fatalf("temp directory path should remain writable on platform aliases: %v", err)
	}
}

func TestTrustedPlatformAliasIsRestrictedToKnownDarwinPaths(t *testing.T) {
	root := string(filepath.Separator)
	if runtime.GOOS == "darwin" {
		for _, path := range []string{"/etc", "/tmp", "/var"} {
			if !isTrustedPlatformAlias(filepath.FromSlash(path), root) {
				t.Fatalf("%s should be a trusted Darwin platform alias", path)
			}
		}
	}
	if isTrustedPlatformAlias(filepath.Join(root, "workspace"), root) {
		t.Fatal("arbitrary root-child symlink must not be trusted")
	}
}

func TestWriteConfigFileUsesPrivatePermissionsAndMayStoreSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not authoritative on Windows")
	}

	root := t.TempDir()
	path := filepath.Join(root, "config", "config.json")
	secretConfig := []byte(`{"delivery":{"slack_webhook_url":"https://hooks.slack.com/services/T000/B000/config-secret"}}`)

	if err := WriteConfigFile(path, secretConfig); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode() & 0o777; got != PrivateDirMode {
		t.Fatalf("config dir mode = %#o, want %#o", got, PrivateDirMode)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode() & 0o777; got != PrivateFileMode {
		t.Fatalf("config file mode = %#o, want %#o", got, PrivateFileMode)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "config-secret") {
		t.Fatalf("config storage should preserve the secret for later defaults")
	}
}
