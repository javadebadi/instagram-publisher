---
layout: default
title: Post from Python
render_with_liquid: false
parent: Tutorial
nav_order: 21
---

# Post from Python

The same call with the standard library only, and a guard that survives the
unclear case.

```python
import json
import pathlib
import urllib.error
import urllib.request

SERVICE = "http://localhost:8080"
RECORD = pathlib.Path("october.posted.json")


def post_carousel(images, caption, *, token_ref, user_id, dry_run=False):
    if RECORD.exists() and not dry_run:
        raise SystemExit(f"already posted: {RECORD.read_text()}")
    body = json.dumps({
        "token_ref": token_ref, "user_id": user_id,
        "images": images, "caption": caption, "dry_run": dry_run,
    }).encode()
    request = urllib.request.Request(
        f"{SERVICE}/publish/carousel", data=body,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=600) as resp:
            result = json.load(resp)
    except urllib.error.HTTPError as err:
        detail = json.load(err)
        if detail.get("outcome_unknown"):
            raise SystemExit(f"MAYBE POSTED, check the account before retrying: {detail['error']}")
        if detail.get("token_error"):
            raise SystemExit(f"the token needs replacing: {detail['error']}")
        raise SystemExit(f"{err.code}: {detail['error']}")
    if not dry_run:
        RECORD.write_text(json.dumps(result))  # before anything else can fail
    return result


print(post_carousel(
    ["https://example.com/1.jpg", "https://example.com/2.jpg"],
    "Hello",
    token_ref="envfile:/home/me/.ig.env#IG_TOKEN",
    user_id="17841400000000000",
    dry_run=True,
))
```

Flip `dry_run` to `False` when the dry run answers cleanly.
