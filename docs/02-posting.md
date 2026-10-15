---
layout: default
title: Posting a carousel
render_with_liquid: false
nav_order: 3
---

# 2. Posting a carousel

Take the dry-run request and remove `"dry_run": true` (or set it to `false`):

```sh
curl -s localhost:8080/publish/carousel -d '{
  "token_ref": "envfile:.env.local#IG_TOKEN",
  "user_id": "YOUR_NUMERIC_USER_ID",
  "images": ["https://example.com/slides/1.jpg", "https://example.com/slides/2.jpg"],
  "caption": "My first post"
}'
```

It answers when the post is live:

```json
{"dry_run":false,"account":"yourname","media_id":"1789...","permalink":"https://www.instagram.com/p/AbC/","published_at":"2026-10-15T09:00:00Z"}
```

This can take a minute or more: Instagram prepares each image and then the
carousel, and the service waits for each step. Do not set a short timeout on the
call.

## The rules of a post

| | |
|---|---|
| images | 1 to 10 `http(s)` addresses; one image makes a single-image post |
| format | JPEG, every address answering `200` with `Content-Type: image/jpeg` |
| shape | Instagram accepts aspect ratios from 4:5 to 1.91:1 |
| caption | up to 2200 characters |
| rate | about 100 posts per 24 hours per account (Instagram's limit) |

A signed (expiring) URL works, even if the host refuses `HEAD` requests, and
the signature is never repeated in error messages.

## You must not post twice

**Instagram's API cannot delete a post**, and the service keeps no memory of what
it posted, so a repeated request is a repeated post. Avoiding that is the
caller's job. The pattern that works:

1. Before calling, check you have no record of having posted this content.
2. Call `/publish/carousel`.
3. On success, write down `media_id` and `permalink` straight away, before
   anything else can fail.
4. If the call fails, read [the errors](03-troubleshooting.md) first. One of
   them, `outcome_unknown`, means *the post may be live*: look at the account
   before trying again.

The [Python example](examples/python.md) does exactly this with a small record
file.
