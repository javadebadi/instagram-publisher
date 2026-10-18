---
layout: default
title: Tutorial
render_with_liquid: false
nav_order: 1
permalink: /
---

# instagram-publisher tutorial

instagram-publisher turns "post these pictures to Instagram" into one HTTP
call. You send it the addresses of images or videos that are already online, a
caption, and which account to post as. It posts them (as a carousel, a reel or a
story) and sends back the link.

It does only that. It does not make the pictures, schedule anything or decide
what to say: whatever produces your media (a design tool, a script, an AI)
stays outside, and this is the last step.

Two ideas make it easy to run anywhere:

- **It keeps nothing.** No database, no stored token. Every call says where the
  account's access token lives, and the service reads it only when it needs it.
- **Tokens are named, not sent.** You pass `env:IG_TOKEN` or
  `gcp:projects/P/secrets/S`, never the token itself. Which places a build can
  read from is a choice you make when you build it.

Read the pages in order the first time; each one adds one idea.

| | Page | You learn |
|---|---|---|
| 1 | [Your first dry run](01-first-dry-run.md) | install it, point it at a token, check an account and some images without posting |
| 2 | [Posting a carousel](02-posting.md) | the real post, what comes back, and what to do so you never post twice |
| 3 | [Posting reels and stories](07-reels-and-stories.md) | the other two kinds of post: videos, covers, and what a story takes |
| 4 | [When something goes wrong](03-troubleshooting.md) | every error the service gives and what to do about it |
| 5 | [Where tokens live](04-secrets.md) | the `scheme:name` references, and adding your own store (AWS, a vault) |
| 6 | [Keeping the token fresh](05-token-refresh.md) | Instagram tokens expire in 60 days; refreshing on a schedule |
| 7 | [Deploying it](06-deploying.md) | a container, who may call it, a Cloud Run example |

## Worked examples

- [Post from a shell script](examples/shell-script.md): a dozen lines that publish a folder of JPEGs you already host.
- [Post from Python](examples/python.md): the same call in code, with the double-post guard.

The [README](https://github.com/javadebadi/instagram-publisher#readme) is the reference; these pages are the walkthrough.

## What it is not

It posts **carousels** (one to ten images; one image makes a single-image post),
**reels** and **stories**, each on its own route. It does not schedule, make
media, or post anything but these; each new kind would be another route added
beside the others without changing them.
