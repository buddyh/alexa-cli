---
name: amazon-package-status
description: >-
  Check your Amazon (and other) package delivery status from the terminal using
  the alexacli CLI — silently, with rich per-package detail (delivery windows,
  carriers, counts). Use when someone asks "where are my packages", "what's
  arriving today/tomorrow", "track my deliveries", or wants to build a package /
  delivery tracking automation. Works by asking Alexa+ over a minted conversation
  id so no Echo speaker ever plays.
---

# Amazon Package Status via alexacli

Get a full, structured rundown of your incoming packages — **silently**, from the
command line — by querying **Alexa+** (Amazon's LLM assistant) through
[`alexacli`](https://github.com/buddyh/alexa-cli). Alexa is tied to your Amazon
account, so it already knows every package on the way.

## TL;DR

```bash
alexacli askplus -c "amzn1.conversation.$(uuidgen | tr 'A-Z' 'a-z')" \
  "List every package arriving today and tomorrow with item count, delivery window, and carrier."
```

That single command returns a markdown breakdown like:

> **Today:** 7 packages / 18 items — 12:45–3:00 PM (Amazon), by 10 PM (UPS)
> **Tomorrow:** 10 packages — by 10 PM (USPS, Amazon)
> **Later:** Tuesday, Thursday

No Echo speaks. No screen-scraping. No Amazon login flow.

## The key technique: mint a conversation id

The non-obvious part is the conversation id. You do **not** need a pre-existing
one — **generate a fresh UUID in Amazon's format and pass it with `-c`:**

```bash
CONV="amzn1.conversation.$(uuidgen | tr 'A-Z' 'a-z')"
alexacli askplus -c "$CONV" "where are my packages?"
```

Amazon accepts a client-generated `amzn1.conversation.<uuid>` and starts a new
Alexa+ session on the spot (the CLI's `InitConversation()` does exactly this).
Reuse the same `$CONV` across calls to keep multi-turn context.

To reuse an existing conversation instead, list them:

```bash
alexacli conversations            # shows id + device ("Alexa App" = the silent ones)
alexacli askplus -c <id> "..."
alexacli fragments <id>           # dump a conversation's messages
```

## Why `askplus`, not `ask`

| | `ask` | `askplus` |
|---|---|---|
| Backend | legacy voice/TextCommand | **Alexa+ LLM** |
| Audio | **speaks on the target Echo** | **silent** (text-only API) |
| Detail | one canned sentence | full per-package tables, reasoning, formatting |
| Needs | a device `-d` | a conversation id `-c` (mint one) |

`alexacli ask "where are my packages" -d Kitchen` works but makes the **Kitchen
Echo talk out loud**, and returns only a one-line summary. For silent, detailed,
scriptable results, use `askplus` with a minted conversation id.

> Heads-up: pointing `ask` at the phone-app "virtual device" is **not** a reliable
> silent path — it uses the legacy backend, doesn't reach Alexa+, and its
> responses don't show up in `alexacli history`. Use `askplus`.

## Prerequisites

1. **Install alexacli**
   ```bash
   brew install buddyh/tap/alexacli
   # or: go install github.com/buddyh/alexa-cli/cmd/alexa@latest
   ```
2. **Authenticate** (obtains an Amazon refresh token; valid ~14 days)
   ```bash
   alexacli auth
   ```
3. **Alexa+** must be enabled on your Amazon account (the `askplus` LLM path).

## Useful queries

```bash
CONV="amzn1.conversation.$(uuidgen | tr 'A-Z' 'a-z')"

# Everything, grouped by day
alexacli askplus -c "$CONV" "List all packages arriving today, tomorrow, and later this week with windows and carriers."

# Just today, just the count
alexacli askplus -c "$CONV" "How many packages arrive today and in what time window?"

# A specific order / brand
alexacli askplus -c "$CONV" "Where is my Philips Hue order and when will it arrive?"

# Machine-readable (great for scripts): ask for JSON
alexacli askplus -c "$CONV" --json \
  'Return ONLY a JSON array of packages arriving today/tomorrow. Each: {"day","window","carrier","items"}.'
```

Add `-t 30` to raise the response timeout for big answers.

## Gotchas

- **Privacy-protected item titles.** Alexa+ often returns "1 item (privacy
  protected)" instead of the product name. Reveal them in the Alexa app →
  Settings → Notifications → Amazon Shopping. Or cross-reference your email
  (Amazon "Shipped:" / "Ordered:" subjects contain the real titles).
- **Conversation ids persist.** Minted ones keep working for a long time; you can
  reuse one across runs for continuity, or mint fresh each run for a clean slate.
- **`ask` is audible; `askplus` is silent.** Don't put `ask` in a background job.
- **Other merchants too.** Alexa only knows Amazon-shipped/Amazon-visible
  packages. For non-Amazon merchants (Home Depot, B&H, etc.), combine this with
  email parsing (order-confirmation / shipment / delivery notifications) for full
  coverage. See `examples/package-digest` for a hybrid email + Alexa+ digest.

## Building an automation

For a hands-off tracker, **Alexa+ supplies the live ETAs** (windows + carriers)
and **email supplies the item names + non-Amazon merchants**. A daily run:

```bash
# 1) live windows from Alexa+ (silent)
alexacli askplus -c "amzn1.conversation.$(uuidgen|tr A-Z a-z)" --json \
  'JSON array of packages arriving today/tomorrow: {"day","window","carrier","items"}'
# 2) merge with shipment/delivery emails parsed from your inbox
# 3) print/notify a combined digest
```

This is silent end-to-end and needs no browser or Amazon login.
