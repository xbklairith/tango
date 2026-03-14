package main

import (
	"bytes"
	"testing"
)

func TestRootCmd_Help(t *testing.T) {
	cmd := newRootCmd("test")
	cmd.SetArgs([]string{"--help"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Ari")) {
		t.Errorf("help output should contain 'Ari', got: %s", output)
	}
}

func TestVersionCmd_PrintsVersion(t *testing.T) {
	cmd := newRootCmd("test-v1.2.3")
	cmd.SetArgs([]string{"version"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("ari version test-v1.2.3")) {
		t.Errorf("version output should contain 'ari version test-v1.2.3', got: %s", output)
	}
}

func TestRunCmd_HasPortFlag(t *testing.T) {
	cmd := newRunCmd("test")
	flag := cmd.Flags().Lookup("port")
	if flag == nil {
		t.Fatal("run command should have --port flag")
	}
}

func TestRunCmd_FailsOnBadConfig(t *testing.T) {
	t.Setenv("ARI_PORT", "invalid")

	cmd := newRootCmd("test")
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should return error for invalid config")
	}
}
