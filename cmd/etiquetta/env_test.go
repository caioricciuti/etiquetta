package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "test.env")
	content := "" +
		"# a comment\n" +
		"\n" +
		"ETIQUETTA_TEST_PLAIN=hello\n" +
		"export ETIQUETTA_TEST_EXPORTED=world\n" +
		"ETIQUETTA_TEST_QUOTED=\"quoted value\"\n" +
		"ETIQUETTA_TEST_SINGLE='single value'\n" +
		"ETIQUETTA_TEST_HASH=has#hash\n" +
		"ETIQUETTA_TEST_PRESET=fromfile\n" +
		"=novalue\n"
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	// A value already in the real environment must win over the file.
	t.Setenv("ETIQUETTA_TEST_PRESET", "fromenv")
	t.Setenv("ETIQUETTA_ENV_FILE", envFile)

	loadDotEnv()

	cases := map[string]string{
		"ETIQUETTA_TEST_PLAIN":    "hello",
		"ETIQUETTA_TEST_EXPORTED": "world",
		"ETIQUETTA_TEST_QUOTED":   "quoted value",
		"ETIQUETTA_TEST_SINGLE":   "single value",
		"ETIQUETTA_TEST_HASH":     "has#hash", // inline # is not a comment
		"ETIQUETTA_TEST_PRESET":   "fromenv",  // real env wins
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// TestLoadDotEnvMissingFileIsNoop confirms an absent file is not fatal.
func TestLoadDotEnvMissingFileIsNoop(t *testing.T) {
	t.Setenv("ETIQUETTA_ENV_FILE", filepath.Join(t.TempDir(), "does-not-exist.env"))
	loadDotEnv() // must not panic or exit
}
