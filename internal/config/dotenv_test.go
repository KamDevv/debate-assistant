package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		line, key, value string
		wantOK           bool
	}{
		{"GEMINI_API_KEY=abc123", "GEMINI_API_KEY", "abc123", true},
		{"export TAVILY_API_KEY='quoted value' # note", "TAVILY_API_KEY", "quoted value", true},
		{`TOKEN="line\\nnext"`, "TOKEN", "line\\nnext", true},
		{"# comment", "", "", false},
	}
	for _, test := range tests {
		key, value, ok, err := parseLine(test.line)
		if err != nil {
			t.Fatalf("parseLine(%q): %v", test.line, err)
		}
		if ok != test.wantOK || key != test.key || value != test.value {
			t.Errorf("parseLine(%q) = (%q, %q, %v), want (%q, %q, %v)", test.line, key, value, ok, test.key, test.value, test.wantOK)
		}
	}
}

func TestLoadFileDoesNotOverrideExistingEnvironment(t *testing.T) {
	t.Setenv("DEBATE_DOTENV_TEST", "from-process")
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	t.Setenv("DEBATE_DOTENV_NEW", "")
	if err := os.WriteFile(path, []byte("DEBATE_DOTENV_TEST=from-file\nDEBATE_DOTENV_NEW=loaded\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DEBATE_DOTENV_TEST"); got != "from-process" {
		t.Fatalf("existing environment was overridden: %q", got)
	}
	if got := os.Getenv("DEBATE_DOTENV_NEW"); got != "loaded" {
		t.Fatalf(".env value was not loaded: %q", got)
	}
}

func TestLoadDotEnvFindsParentDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	root := t.TempDir()
	child := filepath.Join(root, "cmd", "server")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DEBATE_DOTENV_PARENT=found\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEBATE_DOTENV_PARENT", "")
	if err := os.Chdir(child); err != nil {
		t.Fatal(err)
	}
	path, err := LoadDotEnv()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, ".env") {
		t.Fatalf("LoadDotEnv path = %q, want %q", path, filepath.Join(root, ".env"))
	}
	if got := os.Getenv("DEBATE_DOTENV_PARENT"); got != "found" {
		t.Fatalf("parent .env value was not loaded: %q", got)
	}
}
