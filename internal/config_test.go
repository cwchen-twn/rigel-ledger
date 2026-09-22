package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDotEnvDoesNotOverrideEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("RL_TEST_SET=from-file\nRL_TEST_UNSET=\"quoted\" # comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RL_TEST_SET", "from-env")
	os.Unsetenv("RL_TEST_UNSET")
	t.Cleanup(func() { os.Unsetenv("RL_TEST_UNSET") })

	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("RL_TEST_SET"); got != "from-env" {
		t.Fatalf("RL_TEST_SET = %q, want the environment to win", got)
	}
	if got := os.Getenv("RL_TEST_UNSET"); got != "quoted" {
		t.Fatalf("RL_TEST_UNSET = %q, want %q", got, "quoted")
	}
}

func TestDSNEscapesPassword(t *testing.T) {
	c := &Config{PgUser: "u", PgPassword: "p@ss/word", PgHost: "db", PgPort: 5432, PgDbname: "rl"}
	if got, want := c.DSN(), "postgres://u:p%40ss%2Fword@db:5432/rl?sslmode=disable"; got != want {
		t.Fatalf("DSN = %q, want %q", got, want)
	}
	c.DatabaseURL = "postgres://x"
	if c.DSN() != "postgres://x" {
		t.Fatal("DATABASE_URL did not win")
	}
}
