---
layout: default
title: Deploying it
render_with_liquid: false
nav_order: 7
---

# 6. Deploying it

The service is a single static binary with no state, so it runs anywhere:
your laptop, a VM under systemd, a container on any platform.

## Before you expose it

**The service has no authentication.** Anyone who can reach it can post to
any account whose token reference they can name. Put it behind something that
decides who may call it:

- a private network or a loopback address (`--addr 127.0.0.1:8080`),
- your platform's own access control (Cloud Run IAM, a service mesh, a reverse
  proxy with authentication).

The other protection is the store: the service can only read the references its
own credentials allow. Give it access to the tokens it needs and nothing else.

## A container

The `Dockerfile` in the repository builds a static binary into a distroless
image of about 10 MB:

```sh
docker build -t instagram-publisher .
docker run --rm -p 8080:8080 -e IG_TOKEN=... instagram-publisher
# callers then use "token_ref": "env:IG_TOKEN"
```

It listens on `$PORT` (default 8080) or `--addr`.

## An example: Google Cloud Run

Cloud Run gives you authentication (IAM) and a cloud secret store for free:

1. Build the image and push it to Artifact Registry (or use `gcloud builds submit`).
2. Deploy it as a service that **requires authentication**; give the callers
   the `Cloud Run Invoker` role.
3. Give the service's own account `Secret Manager Secret Accessor` on the
   secrets that hold tokens (and `Secret Manager Secret Version Adder` if it
   should refresh them).
4. Callers send `"token_ref": "gcp:projects/PROJECT/secrets/NAME"`.

The tokens never appear in the callers' configuration; they only know the name.

Deploying is yours to automate in your own repository: pin a release of this one
(`go install ...@v0.1.0` in a Dockerfile) and rebuild when you change the pin.
This repository deliberately contains no deployment settings of its own.

## Versions

Releases are tagged `vMAJOR.MINOR.PATCH`; a tag builds binaries for Linux and
macOS (amd64 and arm64) and attaches them with checksums. `instagram-publisher
--version` prints the version of a release build.
