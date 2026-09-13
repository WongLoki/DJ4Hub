package main

import (
	"context"
	"errors"
	"testing"
)

func TestAudioRootAlreadyRoot(t *testing.T) {
	if err := ensureAudioRoot(context.Background(), func() (string, error) { return "0", nil }, func() error { t.Fatal("unexpected root restart"); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestAudioRootDeniedAndUnauthorized(t *testing.T) {
	if err := ensureAudioRoot(context.Background(), func() (string, error) { return "2000", nil }, func() error { return errors.New("denied") }); err == nil {
		t.Fatal("denial ignored")
	}
	if err := ensureAudioRoot(context.Background(), func() (string, error) { return "", errors.New("unauthorized") }, func() error { t.Fatal("must not request root"); return nil }); err == nil {
		t.Fatal("authorization ignored")
	}
}

func TestAudioRootRevalidatesAfterRestart(t *testing.T) {
	root := false
	requests := 0
	err := ensureAudioRoot(context.Background(), func() (string, error) {
		if root {
			return "0", nil
		}
		return "2000", nil
	}, func() error { requests++; root = true; return nil })
	if err != nil || requests != 1 {
		t.Fatalf("requests %d, err %v", requests, err)
	}
}
