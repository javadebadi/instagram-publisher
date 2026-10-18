---
layout: default
title: Keeping the token fresh
render_with_liquid: false
nav_order: 7
---

# 5. Keeping the token fresh

A long-lived Instagram token lasts **60 days**. Before it runs out you can
exchange it for a new 60-day token, but only once it is **at least 24 hours
old**. If it expires, a person has to generate a new one; refreshing in time
means nobody has to.

The service does the exchange and stores the new token where the old one was:

```sh
curl -s localhost:8080/token/refresh -d '{"token_ref": "envfile:.env.local#IG_TOKEN"}'
# {"refreshed_at":"2026-10-15T09:00:00Z","expires_at":"2026-12-14T09:00:00Z"}
```

The answer has only dates. The token is read, exchanged and written back, and
never returned or logged. The store must be writable (`envfile:` and `gcp:`
are; `env:` is not).

## On a schedule

Refresh far more often than every 60 days, since a refresh that fails once then
still has time to be fixed. Twice a month is a comfortable rhythm.

With cron, on the machine running the service:

```cron
0 6 1,15 * * curl -fsS localhost:8080/token/refresh -d '{"token_ref":"envfile:/srv/ig/.env#IG_TOKEN"}'
```

With a cloud scheduler, make it call `POST /token/refresh` on the deployed
service. Each account has its own reference, so schedule one call per account.

A failed refresh answers `502`; if `token_error` is true the token is already
dead and needs to be replaced by hand.
