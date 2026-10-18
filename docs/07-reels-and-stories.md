---
layout: default
title: Posting reels and stories
render_with_liquid: false
nav_order: 4
---

# Posting reels and stories

A reel and a story are posted like a carousel: you send addresses of files that
are already online, and the service posts them and sends back the link. Each
kind has its own route, because each takes different inputs.

| Kind | Route | Takes |
|---|---|---|
| Carousel | `POST /publish/carousel` | 1 to 10 JPEGs and a caption ([page 2](02-posting.md)) |
| Reel | `POST /publish/reel` | one video, a caption, and options for the cover and the feed |
| Story | `POST /publish/story` | one JPEG **or** one video; no caption |

Everything else is the same as for a carousel: `token_ref` and `user_id` name
the account, `dry_run` checks everything and posts nothing, and the answer has
the same shape (`account`, `media_id`, `permalink`, `published_at`). The
[errors](03-troubleshooting.md) are the same too, including the rule that you
**must not post twice**: Instagram's API cannot delete a post, and the service
keeps no record of what it posted.

## A reel

```sh
curl -s localhost:8080/publish/reel -d '{
  "token_ref": "envfile:.env.local#IG_TOKEN",
  "user_id": "YOUR_NUMERIC_USER_ID",
  "video": "https://example.com/reels/launch.mp4",
  "caption": "Launch day",
  "share_to_feed": true,
  "cover": "https://example.com/reels/launch-cover.jpg"
}'
```

| Field | Required | |
|---|---|---|
| `video` | yes | an `http(s)` address answering `200` with `Content-Type` `video/mp4` or `video/quicktime` |
| `caption` | no | up to 2200 characters |
| `share_to_feed` | no | `true` also shows the reel on the profile grid; `false` shows it only in the Reels tab. Left out, Instagram's own default applies, which has changed over time, so set it if you care. |
| `cover` | no | a public JPEG used as the cover picture. Left out, Instagram uses a frame of the video. |
| `thumb_offset_ms` | no | which frame of the video becomes the cover, in milliseconds from the start (`2000` is two seconds in). Use it when you have no cover image. **Not allowed together with `cover`**: the request is refused with `400` rather than silently ignoring one of them. |
| `dry_run` | no | check and post nothing |

Instagram's own limits for video apply: MP4 or MOV with H.264 video and AAC
audio, between 3 seconds and 15 minutes, 9:16 recommended. The service checks
that the address is reachable and has a video type, but it does not look inside
the file; if Instagram rejects the video, its reason comes back in the `502`
message.

**Reels take longer than images.** Instagram has to process the video before it
will publish it, and the service waits up to 10 minutes for that. Do not set a
short timeout on the call.

## A story

A story is one picture or one video, set with `image` or `video` (exactly one):

```sh
curl -s localhost:8080/publish/story -d '{
  "token_ref": "envfile:.env.local#IG_TOKEN",
  "user_id": "YOUR_NUMERIC_USER_ID",
  "image": "https://example.com/stories/today.jpg"
}'
```

```sh
curl -s localhost:8080/publish/story -d '{
  "token_ref": "envfile:.env.local#IG_TOKEN",
  "user_id": "YOUR_NUMERIC_USER_ID",
  "video": "https://example.com/stories/today.mp4"
}'
```

Stories take no caption. An `image` must be a JPEG and a `video` an MP4 or MOV,
checked the same way as above; giving a video as `image` (or the reverse) is
refused with `422` before anything is posted. A story disappears from the
profile after 24 hours, but a second request is still a second story, so the
same care about double-posting applies.

## What it looked like for real

These are posts made with this service to a real account, with the answers it
sent back.

**A story from an image.** A 1080x1920 JPEG (9:16) on a public static host:

```sh
curl -s localhost:8080/publish/story -d '{
  "token_ref": "gcp:projects/MY_PROJECT/secrets/MY_TOKEN",
  "user_id": "17841400000000001",
  "image": "https://example.com/stories/coming-soon.jpg"
}'
```

```json
{"dry_run":false,"account":"yourname","media_id":"1795...","permalink":"https://www.instagram.com/stories/yourname/4010...","published_at":"2026-10-18T07:57:25Z"}
```

It returned within seconds. A story's `permalink` has the form
`/stories/<username>/<id>`, a reel's is `/reel/<code>/`.

**A reel from a long video.** A 4 minute, 19 MB MP4, shown only in the Reels
tab (`share_to_feed: false`):

```sh
curl -s -m 700 localhost:8080/publish/reel -d '{
  "token_ref": "gcp:projects/MY_PROJECT/secrets/MY_TOKEN",
  "user_id": "17841400000000001",
  "video": "https://example.com/reels/intro.mp4",
  "caption": "A short intro",
  "share_to_feed": false
}'
```

```json
{"dry_run":false,"account":"yourname","media_id":"1809...","permalink":"https://www.instagram.com/reel/AbC123xyz/","published_at":"2026-10-18T08:04:07Z"}
```

This call took **several minutes**, because Instagram processes the video
before it will publish it. The service waits for it; your caller must too (the
`-m 700` above is curl's own limit). A caller that gives up early may leave the
reel still being made, and it can still go out: check the account before trying
again.

## Lessons from those first real posts

- **Run a dry run first, with the real files.** It checks the token, the
  account and that every address can be fetched, and posts nothing. Do it before
  the first real post of a new kind, and after changing where the files live.
- **Hosts that refuse unknown programs.** Wikimedia Commons answers `403` to
  clients that do not name themselves. The service names itself in the checks
  it makes, so a Commons picture works as a story `image`.
- **Where the files live is up to you.** Any public `http(s)` address works.
  A bucket that blocks all public access (for example Google Cloud Storage with
  "public access prevention" enforced) cannot serve them to Instagram: use a
  public static host, or a signed URL.
- **A reel cannot be taken back through the API.** Delete it in the Instagram
  app, and keep a record of `media_id` and `permalink` straight away, as for
  every post.

## From Go

The [`client` package](https://github.com/javadebadi/instagram-publisher#from-go-package-client)
has a call for each:

```go
feed := true
reel, err := c.Reel(ctx, "https://example.com/launch.mp4", client.ReelOptions{
	Caption:     "Launch day",
	ShareToFeed: &feed,
	Cover:       "https://example.com/launch-cover.jpg",
})

story, err := c.StoryImage(ctx, "https://example.com/today.jpg")
story, err = c.StoryVideo(ctx, "https://example.com/today.mp4")
```

`ShareToFeed` and `ThumbOffsetMS` are pointers so that "not set" (leave it to
Instagram) can be told apart from `false` and `0`.
