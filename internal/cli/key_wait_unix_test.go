//go:build darwin || linux

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ai-bricklaying/internal/config"
)

func TestPTYCLISecretInputIsHiddenCancelableAndRestoresTerminal(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script command is required for the PTY regression test")
	}

	t.Run("hidden input permits q after the first byte and supports backspace", func(t *testing.T) {
		corrected := "https://hooks.slack.com/services/T000/B000/canary-q-secret"
		input := []byte(strings.TrimSuffix(corrected, "t") + "x\x7ft\r")
		result := runCLIInScriptPTY(t, input)

		if result.exitCode != 0 {
			t.Fatalf("CLI exit = %d, want 0; output=%q", result.exitCode, result.output)
		}
		if !result.restored {
			t.Fatalf("terminal state was not restored; output=%q", result.output)
		}
		if strings.Contains(result.output, "canary-q-secret") || strings.Contains(result.output, "canary-q-secrex") {
			t.Fatalf("PTY echoed hidden secret: %q", result.output)
		}
		var saved config.StoredConfig
		contents, err := os.ReadFile(filepath.Join(result.configDir, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(contents, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.Delivery.SlackWebhookURL != corrected {
			t.Fatalf("saved hidden input = %q, want corrected value", saved.Delivery.SlackWebhookURL)
		}
	})

	for name, input := range map[string][]byte{
		"q on empty input": {'q'},
		"escape":           {0x1b},
		"ctrl-c":           {0x03},
	} {
		t.Run(name, func(t *testing.T) {
			started := time.Now()
			result := runCLIInScriptPTY(t, input)
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Fatalf("cancel was not immediate: %s", elapsed)
			}
			if result.exitCode != contractExit {
				t.Fatalf("CLI exit = %d, want %d; output=%q", result.exitCode, contractExit, result.output)
			}
			if !strings.Contains(result.output, "Setup cancelled.") {
				t.Fatalf("cancel output missing contract message: %q", result.output)
			}
			if !result.restored {
				t.Fatalf("terminal state was not restored; output=%q", result.output)
			}
		})
	}
}

// TestPTYCLIHelperProcess runs only inside script(1), which supplies a real
// controlling pseudo-terminal to os.Stdin and os.Stdout.
func TestPTYCLIHelperProcess(t *testing.T) {
	if os.Getenv("AI_BRICKLAYING_PTY_HELPER") != "1" {
		return
	}
	before, err := terminalStateForTest(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stdout, "PTY_HELPER_ERROR:%s\r\n", err)
		return
	}
	terminalSecretReady = func() {
		fmt.Fprint(os.Stdout, "\r\nRAW_SECRET_READY\r\n")
	}
	exitCode := Run([]string{"--config-dir", os.Getenv("AI_BRICKLAYING_PTY_CONFIG_DIR")}, os.Stdout, os.Stderr)
	terminalSecretReady = nil
	after, stateErr := terminalStateForTest(int(os.Stdin.Fd()))
	restored := stateErr == nil && terminalStatesEqualForTest(before, after)
	fmt.Fprintf(os.Stdout, "\r\nTERMINAL_RESTORED:%t\r\nCLI_EXIT:%d\r\n", restored, exitCode)
}

type scriptPTYResult struct {
	configDir string
	exitCode  int
	output    string
	restored  bool
}

func runCLIInScriptPTY(t *testing.T, secretInput []byte) scriptPTYResult {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, map[string]any{
		"defaults": map[string]any{
			"target_agents": []string{"opencode"},
			"source":        "opencode",
			"language":      "English",
			"output_modes":  []string{"file", "slack-webhook"},
			"skill_name":    "pty-secret-test",
			"skill_dir":     filepath.Join(root, "skills"),
			"output_dir":    filepath.Join(root, "out"),
		},
	})

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := scriptPTYCommand(ctx, executable)
	cmd.Env = append(os.Environ(),
		"AI_BRICKLAYING_PTY_HELPER=1",
		"AI_BRICKLAYING_PTY_CONFIG_DIR="+configDir,
		"NO_COLOR=1",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var commandStderr bytes.Buffer
	cmd.Stderr = &commandStderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(stdout)
	var captured bytes.Buffer
	drive := func(pattern string, input []byte) {
		t.Helper()
		if err := readPTYUntil(reader, &captured, pattern); err != nil {
			_ = cmd.Process.Kill()
			t.Fatalf("waiting for %q: %v; output=%q stderr=%q", pattern, err, captured.String(), commandStderr.String())
		}
		if _, err := stdin.Write(input); err != nil {
			_ = cmd.Process.Kill()
			t.Fatalf("writing input after %q: %v", pattern, err)
		}
	}
	drive("1. Select target AI agents", []byte{'\r'})
	drive("2. Select one AI agent", []byte{'\r'})
	drive("3. Result language", []byte{'\n'})
	drive("4. File save directory", []byte{'\n'})
	drive("5. Select output modes", []byte{'\r'})
	drive("RAW_SECRET_READY", secretInput)
	_ = stdin.Close()
	_, _ = io.Copy(&captured, reader)
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		t.Fatalf("PTY helper timed out; output=%q stderr=%q", captured.String(), commandStderr.String())
	}
	if waitErr != nil {
		t.Fatalf("PTY helper failed: %v; output=%q stderr=%q", waitErr, captured.String(), commandStderr.String())
	}

	output := captured.String() + commandStderr.String()
	return scriptPTYResult{
		configDir: configDir,
		exitCode:  parsePTYInt(output, "CLI_EXIT:"),
		output:    output,
		restored:  strings.Contains(output, "TERMINAL_RESTORED:true"),
	}
}

func scriptPTYCommand(ctx context.Context, executable string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.CommandContext(ctx, "script", "-q", "/dev/null", executable, "-test.run=^TestPTYCLIHelperProcess$")
	}
	command := shellQuote(executable) + " -test.run=^TestPTYCLIHelperProcess$"
	return exec.CommandContext(ctx, "script", "-qefc", command, "/dev/null")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func readPTYUntil(reader *bufio.Reader, captured *bytes.Buffer, pattern string) error {
	for !strings.Contains(captured.String(), pattern) {
		current, err := reader.ReadByte()
		if err != nil {
			return err
		}
		captured.WriteByte(current)
	}
	return nil
}

func parsePTYInt(output string, prefix string) int {
	index := strings.LastIndex(output, prefix)
	if index < 0 {
		return -1
	}
	value := output[index+len(prefix):]
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return -1
	}
	return parsed
}
