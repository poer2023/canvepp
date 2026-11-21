package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestFundingWindowsCredentialRestart(t *testing.T) {
	if dir := os.Getenv("CANVEPP_TEST_REOPEN_KEY"); dir != "" {
		key, err := newOpenAIClient(dir).key()
		if err != nil || key != "test-only" {
			t.Fatal("fixture key could not be restored")
		}
		return
	}
	dir := t.TempDir()
	c := newOpenAIClient(dir)
	if err := c.secrets.Set(c.secretID, []byte("test-only")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.secrets.Delete(c.secretID); err != nil {
			t.Error(err)
		}
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "-test.run=^TestFundingWindowsCredentialRestart$")
	child.Env = append(os.Environ(), "CANVEPP_TEST_REOPEN_KEY="+dir)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("reopen: %s %v", out, err)
	}
}
