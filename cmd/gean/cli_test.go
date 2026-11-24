package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"help", []string{"--help"}, 0, "no commands yet", ""},
		{"help word", []string{"help"}, 0, "no commands yet", ""},
		{"unknown command", []string{"beacon"}, 2, "", `unknown command "beacon"`},
		{"no command", nil, 2, "", "no commands yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runCommand(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Fatalf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
