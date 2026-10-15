// Package secret reads and updates secrets by reference, so the code that
// needs a token never knows where it lives. A reference is "scheme:rest":
//
//	env:NAME                     an environment variable (read only)
//	envfile:/path/.env#KEY       a KEY=value line in an env file (read, update)
//
// Cloud stores live in their own packages (secret/gcp) and are registered by
// the program that starts the service, so a binary carries only the clouds it
// uses. Adding another (AWS, a vault) is one more package and one more
// Register call; nothing that calls Get or Set changes. Values are never put into
// error messages.
package secret

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrReadOnly means the store behind a reference cannot be updated.
var ErrReadOnly = errors.New("this kind of secret cannot be updated")

// Store is one kind of place secrets live. rest is the reference without its
// scheme.
type Store interface {
	Get(ctx context.Context, rest string) (string, error)
	Set(ctx context.Context, rest, value string) error
}

// Resolver finds the store for a reference.
type Resolver struct {
	stores map[string]Store
}

// NewResolver returns a resolver with the built-in env and envfile schemes.
// Add others with Register.
func NewResolver() *Resolver {
	r := &Resolver{stores: map[string]Store{}}
	r.Register("env", envStore{})
	r.Register("envfile", envFileStore{})
	return r
}

// Register adds (or replaces) a scheme.
func (r *Resolver) Register(scheme string, s Store) { r.stores[scheme] = s }

func (r *Resolver) split(ref string) (Store, string, error) {
	scheme, rest, ok := strings.Cut(ref, ":")
	if !ok || scheme == "" || rest == "" {
		return nil, "", fmt.Errorf("secret reference %q: want scheme:name, for example env:IG_ACCESS_TOKEN", ref)
	}
	s, ok := r.stores[scheme]
	if !ok {
		return nil, "", fmt.Errorf("secret reference %q: no %q store (have: %s)", ref, scheme, strings.Join(r.schemes(), ", "))
	}
	return s, rest, nil
}

func (r *Resolver) schemes() []string {
	var out []string
	for name := range r.stores {
		out = append(out, name)
	}
	return out
}

// Get returns the secret's current value. An empty value is an error: a
// secret that exists but holds nothing is a mistake worth saying out loud.
func (r *Resolver) Get(ctx context.Context, ref string) (string, error) {
	s, rest, err := r.split(ref)
	if err != nil {
		return "", err
	}
	v, err := s.Get(ctx, rest)
	if err != nil {
		return "", fmt.Errorf("reading secret %s: %w", ref, err)
	}
	if v == "" {
		return "", fmt.Errorf("secret %s is empty", ref)
	}
	return v, nil
}

// Set replaces the secret's value (a store that versions values adds one).
func (r *Resolver) Set(ctx context.Context, ref, value string) error {
	s, rest, err := r.split(ref)
	if err != nil {
		return err
	}
	if err := s.Set(ctx, rest, value); err != nil {
		return fmt.Errorf("updating secret %s: %w", ref, err)
	}
	return nil
}
