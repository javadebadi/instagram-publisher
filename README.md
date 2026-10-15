# instagram-publisher

A small, stateless HTTP service that publishes to Instagram. A caller sends
image URLs and a caption, says which account to post as and where that
account's access token lives; the service posts and answers with the link.

- **Only publishes.** No rendering, no scheduling, no content. Anything that
  has images at public URLs can use it.
- **Holds no token and no state.** Every call names its token by reference
  (below); the value is read when needed.
- **One route per kind of post.** Carousels today. Reels and stories would be
  `POST /publish/reel` and `POST /publish/story`, each with its own request
  and response, added without touching the others.
- **Pluggable secret stores.** `env:` and `envfile:` are built in; Google
  Secret Manager is a separate package; another cloud is one more package.
- **Small image.** A static binary on distroless, about 10 MB.

Instagram's API cannot delete posts. The service only does what it is asked:
**not posting twice is the caller's job.**

## API

No authentication of its own: run it where something else decides who may call
it (Cloud Run IAM, a private network).

### `POST /publish/carousel`

```json
{"token_ref": "gcp:projects/P/secrets/S", "user_id": "17841400000000001",
 "images": ["https://.../1.jpg", "https://.../2.jpg"], "caption": "...",
 "dry_run": false}
```

Checks the token belongs to `user_id`, checks every image URL is fetchable as
`image/jpeg` (a signed URL works), then posts a carousel (one image posts a
single image). Answers `{account, media_id, permalink, published_at}`. With
`dry_run` it stops before posting and posts nothing. `POST /publish` is the
same route, kept for older callers.

### `POST /account/check`

`{"token_ref": "...", "user_id": "..."}` checks the token works and belongs to
`user_id`, posting nothing. Answers `{"account": "<username>"}`. Use it before
doing work (such as uploading slides) that only makes sense if posting can
succeed.

### `POST /token/refresh`

`{"token_ref": "..."}` refreshes the 60-day Instagram token and stores the new
one at the same reference. It answers only dates, never the token. Run it on a
schedule.

### `GET /health`

### Errors

Bad input answers 400; a token for another account 409; an image that isn't
fetchable 422. A failure of Instagram or of the secret store answers 502 with
`"token_error": true` when the token itself was refused (needs a person, not a
retry) and `"outcome_unknown": true` when the post may have gone out although
Instagram did not answer: **do not retry before looking at the account.**

## Where a token lives: secret references

A token is never sent by value; it is a reference, `scheme:rest`:

| reference | meaning |
|---|---|
| `env:NAME` | an environment variable (read only) |
| `envfile:/path/.env#KEY` | a line of an env file (read and update) |
| `gcp:projects/P/secrets/S` | Google Secret Manager, latest version (update adds a version) |

Which schemes a build understands is decided in
`cmd/instagram-publisher/main.go`, which registers them:

```go
r := secret.NewResolver()   // env:, envfile:
r.Register("gcp", gcp.New(nil))
```

A Go binary contains only the packages its `main` imports, so a build that
doesn't register `gcp` doesn't carry it. To run on another cloud, copy that
file, import your store and register it (a store is two methods, `Get` and
`Set`, see `secret.Store`). `publisher` and `secret` are ordinary public
packages, so your `main` can live in your own repository.

## Install

```sh
go install github.com/javadebadi/instagram-publisher/cmd/instagram-publisher@latest
```

or download a binary for your platform from the
[releases](https://github.com/javadebadi/instagram-publisher/releases) page, or
build the container image from the `Dockerfile` (a static binary on distroless,
about 10 MB). It reads `PORT` (default 8080) or `--addr`.

## Run it

```sh
instagram-publisher serve --addr 127.0.0.1:8080
```

Point a token at an env file and no cloud is involved:

```sh
echo 'IG_TOKEN=...' > .env.local
curl -s localhost:8080/publish/carousel -d '{"token_ref":"envfile:.env.local#IG_TOKEN","user_id":"...","images":["https://..."],"dry_run":true}'
```

Use `dry_run` first: it checks the token, the account and every image URL and
posts nothing.

Where and how you host it (a container platform, a VM, your laptop) is up to
you; the service has no authentication of its own, so put it behind whatever
decides who may call it.

## Layout

```
cmd/instagram-publisher/   main: which secret stores this build has
publisher/                 the HTTP service (service.go shared; carousel.go, refresh.go per route)
secret/, secret/gcp/       references and stores
internal/instagram/        the Instagram API client (Instagram Login, graph.instagram.com)
internal/imagecheck/       is a URL a fetchable JPEG
```

Tests run against a fake Instagram; none of them touch the real API.

## Documentation

A step-by-step tutorial, from a first dry run to deploying and refreshing
tokens, is at https://javadebadi.github.io/instagram-publisher/ (source in
`docs/`).
