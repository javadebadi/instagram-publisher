package secret

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type envStore struct{}

func (envStore) Get(_ context.Context, name string) (string, error) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("environment variable %s is not set", name)
	}
	return v, nil
}

func (envStore) Set(context.Context, string, string) error { return ErrReadOnly }

// envFileStore reads and updates one KEY=value line of an env file:
// "envfile:/path/to/.env#KEY".
type envFileStore struct{}

func splitFileKey(rest string) (path, key string, err error) {
	path, key, ok := strings.Cut(rest, "#")
	if !ok || path == "" || key == "" {
		return "", "", errors.New("want envfile:/path/to/file#KEY")
	}
	return path, key, nil
}

func (envFileStore) Get(_ context.Context, rest string) (string, error) {
	path, key, err := splitFileKey(rest)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return unquote(strings.TrimSpace(value)), nil
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s has no %s= line", path, key)
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

func (envFileStore) Set(_ context.Context, rest, value string) error {
	path, key, err := splitFileKey(rest)
	if err != nil {
		return err
	}
	return ReplaceEnv(path, key, value)
}

// ReplaceEnv sets KEY=value in an env file, replacing the existing line.
func ReplaceEnv(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(?m)^(export\s+)?` + regexp.QuoteMeta(key) + `=.*$`)
	if !re.Match(data) {
		return fmt.Errorf("%s has no %s= line to replace", path, key)
	}
	updated := re.ReplaceAllFunc(data, func(line []byte) []byte {
		prefix := ""
		if strings.HasPrefix(string(line), "export") {
			prefix = "export "
		}
		return []byte(prefix + key + "=" + value)
	})
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, st.Mode().Perm())
}
