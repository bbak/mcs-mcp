package config

import (
	"os"
	"testing"

	"github.com/joho/godotenv"
)

func TestGodotenvQuoting(t *testing.T) {
	content := `TEST_VAR='value with "double quotes"'`
	tmpfile, err := os.CreateTemp("", ".env.test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	env, err := godotenv.Read(tmpfile.Name())
	if err != nil {
		t.Fatalf("Error reading env: %v", err)
	}

	expected := `value with "double quotes"`
	if env["TEST_VAR"] != expected {
		t.Errorf("Expected %s, got %s", expected, env["TEST_VAR"])
	}
}
