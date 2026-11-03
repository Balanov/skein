package main

import "testing"

func TestVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("version exit code = %d", code)
	}
}

func TestUnknownCommand(t *testing.T) {
	if code := run([]string{"unknown"}); code != 2 {
		t.Fatalf("unknown command exit code = %d", code)
	}
}
