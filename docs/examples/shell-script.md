---
layout: default
title: Post from a shell script
render_with_liquid: false
parent: Tutorial
nav_order: 20
---

# Post from a shell script

Your images are already online (here: numbered files on a web server). The
script posts them as a carousel and writes down the result.

```sh
#!/bin/sh
set -eu

BASE=https://example.com/slides/2026-10      # where 1.jpg, 2.jpg, 3.jpg are
RECORD=2026-10.posted.json                   # proof we posted this one

[ -e "$RECORD" ] && { echo "already posted: $(cat "$RECORD")"; exit 1; }

curl -fsS --max-time 600 localhost:8080/publish/carousel -d @- <<JSON | tee "$RECORD"
{
  "token_ref": "envfile:$HOME/.ig.env#IG_TOKEN",
  "user_id": "17841400000000000",
  "images": ["$BASE/1.jpg", "$BASE/2.jpg", "$BASE/3.jpg"],
  "caption": "October, in three pictures."
}
JSON
```

Three details worth copying:

- the record file is the double-post guard: it exists only after a success
  (`tee` writes the answer), and the script refuses to run again if it is there;
- `--max-time 600` is generous on purpose, because posting waits on Instagram;
- `-f` makes curl fail on a `4xx`/`5xx`. On a failure the record is empty or
  absent, so read the error before deleting anything, and remember
  `outcome_unknown` ([troubleshooting](../03-troubleshooting.md)): a `502` with
  that flag means look at the account first.

Try it with `"dry_run": true` in the body first.
