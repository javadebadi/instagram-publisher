---
layout: default
title: Your first dry run
render_with_liquid: false
nav_order: 2
---

# 1. Your first dry run

You will start the service, tell it where a token is, and ask it to check an
account and some image addresses **without posting anything**.

## What you need

- An Instagram **professional** account (Business or Creator).
- A long-lived access token for it, from your own Meta app using
  [Instagram API with Instagram Login](https://developers.facebook.com/docs/instagram-platform/instagram-api-with-instagram-login).
  The token needs the permissions `instagram_business_basic` and
  `instagram_business_content_publish`. Getting one is Meta's side of the work
  and their dashboard changes; their documentation is the place to follow it.
- The account's numeric **user id** (Meta's token tools show it, and the service
  will tell you if the token belongs to a different one).
- One or more JPEG images reachable by a public `https` address. Instagram's
  servers fetch them, so a file on your laptop will not do.

## Install

Either of these:

```sh
go install github.com/javadebadi/instagram-publisher/cmd/instagram-publisher@latest
```

or download the binary for your system from the
[releases page](https://github.com/javadebadi/instagram-publisher/releases)
and make it executable.

## Put the token where the service can read it

For a first try, an env file is simplest. Save this as `.env.local`:

```
IG_TOKEN=paste-the-long-lived-token-here
```

Keep that file out of git. (Page 4 shows where else a token can live.)

## Start the service

```sh
instagram-publisher serve --addr 127.0.0.1:8080
```

Check it is up:

```sh
curl localhost:8080/health
# ok
```

## Check the account

```sh
curl -s localhost:8080/account/check -d '{
  "token_ref": "envfile:.env.local#IG_TOKEN",
  "user_id": "YOUR_NUMERIC_USER_ID"
}'
# {"account":"yourname"}
```

`token_ref` is the pointer to the token, not the token: `envfile:` means "a line
of a file", then the path, then `#` and the name of the line. The service reads
the file, asks Instagram who the token belongs to, and compares it with the
`user_id` you gave. If you gave the wrong account it answers `409` instead of
posting to the wrong place later.

## Dry-run a carousel

```sh
curl -s localhost:8080/publish/carousel -d '{
  "token_ref": "envfile:.env.local#IG_TOKEN",
  "user_id": "YOUR_NUMERIC_USER_ID",
  "images": ["https://example.com/slides/1.jpg", "https://example.com/slides/2.jpg"],
  "caption": "My first post",
  "dry_run": true
}'
# {"dry_run":true,"account":"yourname","message":"dry run: would post 2 slide(s) to @yourname; nothing was posted"}
```

`dry_run` does everything except the post: it reads the token, checks the
account and fetches every image address the way Instagram will (it must answer
`200` and be served as `image/jpeg`). If a link is private, missing or served as
the wrong type you find out here, with the address in the message.

Nothing is posted and nothing is stored. Next: [post for real](02-posting.md).
