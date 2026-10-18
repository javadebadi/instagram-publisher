---
layout: default
title: When something goes wrong
render_with_liquid: false
nav_order: 5
---

# 3. When something goes wrong

Every failure is JSON with an `error` message. The status code says whose
problem it is.

| Status | Meaning | What to do |
|---|---|---|
| `400` | The request is wrong: no `token_ref`, a `user_id` that is not a number, no images or more than 10, an address that is not `http(s)`, a caption over 2200 characters; for [reels and stories](07-reels-and-stories.md): no `video`, a story with both or neither of `image` and `video`, `cover` together with `thumb_offset_ms` | Fix the request; the message names the field. |
| `409` | The token works but belongs to another account than `user_id` | You are pointing at the wrong token or the wrong id. Nothing was posted. |
| `422` | An image, video or cover address could not be used: not reachable, not `200`, or the wrong type (`image/jpeg` for images and covers, `video/mp4` or `video/quicktime` for videos) | Make the file public and served with the right type. If the files are on a host that publishes on a delay (a synced folder), wait for it. Nothing was posted. |
| `502` | Instagram, or the place the token lives, failed | Read the two flags below. |

## The two flags on a 502

```json
{"error":"posting to Instagram: ...","token_error":false,"outcome_unknown":true}
```

**`token_error: true`** means Instagram refused the token (expired, revoked,
missing permission). Retrying will not help. Generate a new token, or if it
only expired, see [refreshing](05-token-refresh.md) for how to avoid it next
time.

**`outcome_unknown: true`** means the connection broke at the moment of
publishing, or Instagram answered with an anti-spam restriction after taking
the post. **The post may be live.** Open the account and look. If it is there,
record it as posted. Only post again if it is not.

If both are `false` and the message says the secret store failed (a file is
missing, a key is not set, a cloud call was denied), the token was never read
and nothing was posted; fix the reference or its permissions.

## Common messages

| Message | Cause |
|---|---|
| `environment variable X is not set` | `env:X` names a variable the service does not have |
| `secret ... is empty` | the file line or secret exists but holds nothing |
| `no "gcp" store (have: ...)` | this build does not include that store ([page 4](04-secrets.md)) |
| `The access token is invalid or expired` | `token_error`; get or refresh a token |
| `Permission missing` | the token needs `instagram_business_basic` and `instagram_business_content_publish` |
| `Instagram could not fetch an image` | an image address is not public, or is not JPEG |
| `The image format or aspect ratio is not accepted` | JPEG, 4:5 to 1.91:1 |
| `reached its publishing limit` | about 100 API posts per 24 hours |

Use `dry_run` to find most of these before anything is at stake.
