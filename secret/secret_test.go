package secret

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnv(t *testing.T) {
	t.Setenv("IGPUB_TEST_SECRET", "s3cret")
	r := NewResolver()
	got, err := r.Get(context.Background(), "env:IGPUB_TEST_SECRET")
	if err != nil || got != "s3cret" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := r.Get(context.Background(), "env:IGPUB_TEST_NOT_SET"); err == nil {
		t.Error("an unset variable should be an error")
	}
	t.Setenv("IGPUB_TEST_EMPTY", "")
	if _, err := r.Get(context.Background(), "env:IGPUB_TEST_EMPTY"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("an empty value should be called out, got %v", err)
	}
	if err := r.Set(context.Background(), "env:IGPUB_TEST_SECRET", "x"); err == nil {
		t.Error("env can't be updated")
	}
}

func TestEnvFileReadAndUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("OTHER=1\nexport TOKEN=\"old value\"\nLAST=2\n"), 0o600)
	r := NewResolver()
	ref := "envfile:" + path + "#TOKEN"
	got, err := r.Get(context.Background(), ref)
	if err != nil || got != "old value" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := r.Set(context.Background(), ref, "new"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "OTHER=1\nexport TOKEN=new\nLAST=2\n" {
		t.Errorf("file is now %q", data)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode changed to %v", st.Mode().Perm())
	}
	if _, err := r.Get(context.Background(), "envfile:"+path+"#MISSING"); err == nil {
		t.Error("a missing key should be an error")
	}
	if _, err := r.Get(context.Background(), "envfile:"+path); err == nil {
		t.Error("a reference without #KEY should be an error")
	}
}

func TestBadReferences(t *testing.T) {
	r := NewResolver()
	for _, ref := range []string{"", "plain", ":x", "env:", "vault:x"} {
		if _, err := r.Get(context.Background(), ref); err == nil {
			t.Errorf("%q should be an error", ref)
		}
	}
}
