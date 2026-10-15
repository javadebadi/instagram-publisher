---
layout: default
title: Where tokens live
render_with_liquid: false
nav_order: 5
---

# 4. Where tokens live

The service never receives a token. It receives a **reference**, `scheme:name`,
and the scheme says which store to ask. This is why one running service can
post for several accounts, and why you can move from one cloud to another
without changing what the callers send.

| Reference | Meaning | Read | Update |
|---|---|---|---|
| `env:NAME` | an environment variable of the service | yes | no |
| `envfile:/path/.env#KEY` | one `KEY=value` line of a file | yes | yes |
| `gcp:projects/P/secrets/S` | Google Secret Manager, latest version | yes | adds a version |

`env:` and `envfile:` are always there. `gcp:` is there in the released binaries
and in `go install` of `cmd/instagram-publisher`.

Updating matters for [token refresh](05-token-refresh.md): the refreshed token
is written back to the same reference, so a read-only store (`env:`) can post
but cannot be refreshed by the service.

## Why only some stores are in your binary

A Go program contains only what it imports. Each cloud store is its own package
(`secret/gcp`), and the program that starts the service lists the stores it
wants. A build without the Google package does not contain it.

This is `cmd/instagram-publisher/main.go`, in full where it matters:

```go
func secrets() *secret.Resolver {
	r := secret.NewResolver() // env:, envfile:
	r.Register("gcp", gcp.New(nil))
	return r
}
```

## Adding your own store

A store is two methods:

```go
type Store interface {
	Get(ctx context.Context, rest string) (string, error)
	Set(ctx context.Context, rest, value string) error
}
```

`rest` is everything after the first colon. Return `secret.ErrReadOnly` from
`Set` if the store cannot be updated. Then write your own small `main`:

```go
package main

import (
	"net/http"

	"github.com/javadebadi/instagram-publisher/publisher"
	"github.com/javadebadi/instagram-publisher/secret"
)

func main() {
	r := secret.NewResolver()
	r.Register("aws", awsStore{}) // your type: Get and Set against AWS Secrets Manager
	http.ListenAndServe(":8080", publisher.New(publisher.Config{Secrets: r}))
}
```

`publisher` and `secret` are ordinary public packages, so this file can live in
your own repository, and callers then send `"token_ref": "aws:my/secret/name"`.
Values are never put into error messages; keep that property in your store.
