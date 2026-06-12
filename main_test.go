package main

import "testing"

func TestVersionOutput(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "v0.1.0", "abc1234", "2026-06-12T00:00:00Z"
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	})

	got := versionOutput()
	want := "git-remote-confluence v0.1.0\ncommit: abc1234\nbuilt: 2026-06-12T00:00:00Z\n"
	if got != want {
		t.Fatalf("versionOutput() = %q, want %q", got, want)
	}
}
