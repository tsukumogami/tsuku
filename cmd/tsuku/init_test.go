package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/project"
)

func TestInitCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir, false); err != nil {
		t.Fatalf("runInit() unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, project.ConfigFileName))
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}

	got := string(data)
	if got != configTemplate {
		t.Errorf("config content mismatch:\ngot:\n%s\nwant:\n%s", got, configTemplate)
	}
}

func TestInitAlreadyExists(t *testing.T) {
	dir := t.TempDir()

	// Create an existing config file.
	existing := filepath.Join(dir, project.ConfigFileName)
	if err := os.WriteFile(existing, []byte("existing"), 0644); err != nil {
		t.Fatalf("writing existing file: %v", err)
	}

	err := runInit(dir, false)
	if err == nil {
		t.Fatal("runInit() expected error for existing file, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}

func TestInitForceOverwrite(t *testing.T) {
	dir := t.TempDir()

	// Create an existing config file with different content.
	existing := filepath.Join(dir, project.ConfigFileName)
	if err := os.WriteFile(existing, []byte("old content"), 0644); err != nil {
		t.Fatalf("writing existing file: %v", err)
	}

	if err := runInit(dir, true); err != nil {
		t.Fatalf("runInit() with force unexpected error: %v", err)
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}

	got := string(data)
	if got != configTemplate {
		t.Errorf("force overwrite content mismatch:\ngot:\n%s\nwant:\n%s", got, configTemplate)
	}
}
