package safeio

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
