# alexa-cli

A command-line interface for controlling Amazon Alexa devices using the unofficial Alexa API.

**Control your smart home from the terminal.** Turn lights on/off, adjust thermostats, lock doors, play music, make announcements - anything you can say to Alexa, you can script.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install buddyh/tap/alexacli
```

This installs both `alexacli` and the companion `alexa-poller` binary.

### Go

```bash
go install github.com/buddyh/alexa-cli/cmd/alexa@latest
go install github.com/buddyh/alexa-cli/cmd/alexa-poller@latest
```

### Build locally

```bash
git clone https://github.com/buddyh/alexa-cli
cd alexa-cli
make build
./bin/alexacli --help

make build-poller
./bin/alexa-poller --help
```

## Authentication

This CLI uses Amazon's unofficial API (the same one the Alexa app uses). Authentication requires an Amazon refresh token, which `alexacli auth` can obtain automatically.

### Browser Login (Recommended)

```bash
alexacli auth
```

This downloads [alexa-cookie-cli](https://github.com/adn77/alexa-cookie-cli) on first run, opens a browser at http://127.0.0.1:8080 for Amazon login, and automatically captures and saves your refresh token.

For non-US accounts (recommended split-domain login flow):

```bash
# Base auth domain (.com) + local marketplace country page (.it/.de/.co.uk...)
alexacli auth --domain amazon.com --country amazon.it
alexacli auth --domain amazon.com --country amazon.de
alexacli auth --domain amazon.com --country amazon.co.uk
```

### Manual Token

```bash
# Provide token directly
alexacli auth <your-refresh-token>

# Or set environment variable
export ALEXA_REFRESH_TOKEN=<your-token>
```

### Credential Management

```bash
# Check auth status
alexacli auth status
alexacli auth status --verify    # also validate token against the API

# Remove stored credentials
alexacli auth logout
```

Configuration is stored in `~/.alexa-cli/config.json`.

### Config fields

```json
{
  "refresh_token": "Atnr|...",
  "amazon_domain": "amazon.com",
  "amazon_local": "amazon.it",
  "default_device": "YOUR_DEVICE_SERIAL"
}
```

- `amazon_domain`: auth/token domain (usually `amazon.com`)
- `amazon_local`: local marketplace/runtime domain (e.g. `amazon.it`, `amazon.de`)
- If `amazon_local` is omitted, the CLI falls back to `amazon_domain`.

## Usage

### List Devices

```bash
# List all Echo devices
alexacli devices

# JSON output for scripting
alexacli devices --json
```

### Text-to-Speech

```bash
# Speak on a specific device
alexacli speak "Hello world" -d "Kitchen Echo"

# Announce to ALL devices
alexacli speak "Dinner is ready!" --announce

# Device name matching is flexible
alexacli speak "Build complete" -d Kitchen
alexacli speak "Build complete" -d "Living Room"
```

### Voice Commands (Smart Home Control)

Send any command as if you spoke it to Alexa. This is the primary way to control smart home devices:

```bash
# Smart home control - lights, switches, plugs
alexacli command "turn off the living room lights" -d Kitchen
alexacli command "turn on the porch light" -d Kitchen
alexacli command "dim the bedroom lights to 50 percent" -d Bedroom

# Thermostats and climate
alexacli command "set thermostat to 72 degrees" -d Bedroom
alexacli command "what's the temperature inside" -d Kitchen

# Locks and security
alexacli command "lock the front door" -d Kitchen

# Play music
alexacli command "play jazz music" -d "Living Room"
alexacli command "stop" -d "Living Room"

# Ask questions
alexacli command "what's the weather" -d Kitchen

# Timers and reminders
alexacli command "set a timer for 10 minutes" -d Kitchen
```

The `-d` flag specifies which Echo device processes the command. The device itself doesn't need to be near the smart home device - Alexa routes the command appropriately.

### Ask (Get Response Back)

Send a command and capture Alexa's text response:

```bash
# Query and get the response
alexacli ask "what's the thermostat set to" -d Kitchen
# Output: The thermostat is set to 68 degrees.

alexacli ask "what's on my calendar today" -d Kitchen
# Output: You have 2 events today. First, standup at 9 AM...

# JSON output for parsing
alexacli ask "what time is it" -d Kitchen --json
# {"data":{"device":"Kitchen","question":"what time is it","response":"The time is 3:22 PM."},"success":true}
```

This retrieves Alexa's actual response by polling the voice activity history. Useful for:
- Querying smart home device state
- Getting Alexa-specific information (calendar, reminders, timers)
- Verifying that commands worked

### History

View recent voice activity:

```bash
alexacli history
alexacli history --limit 5
alexacli history --json
```

Shows what was said and what Alexa responded with.

### Alexa+ (LLM Conversations)

Interact with Alexa+ (Amazon's LLM-powered assistant) via text. Alexa+ is Amazon's newer LLM-powered backend that provides conversational AI responses.

#### Quick Start (Recommended)

```bash
# Just specify the device - conversation ID is auto-selected
alexacli askplus -d "Echo Show" "What's the capital of France?"

# Multi-turn conversations retain context
alexacli askplus -d "Echo Show" "What about Germany?"
```

The `-d` flag automatically finds the most recent conversation for that device.

#### List Conversations

```bash
# See all Alexa+ conversations with devices and last activity
alexacli conversations
```

Output:
```
Found 12 Alexa+ conversation(s):

  Device: Echo Show
  ID:     amzn1.conversation.b2925036-7d5b-461f-a989-04574eb6f2c9
  Last:   2026-01-14 10:30:15

  Device: Kitchen
  ID:     amzn1.conversation.f7277f37-6a7b-4bee-b686-7f3be990f44d
  Last:   2026-01-13 18:22:03
  ...
```

#### Advanced: Use Specific Conversation ID

```bash
# If you need a specific conversation thread
alexacli askplus -c "amzn1.conversation.xxx" "Hello"
```

#### View Conversation History

```bash
# See the full conversation thread
alexacli fragments "amzn1.conversation.xxx"
```

Output shows both your messages (USER) and Alexa's responses (ALEXA):
```
[2026-01-14 10:30:15] USER
  What's the capital of France?

[2026-01-14 10:30:17] ALEXA
  The capital of France is Paris...
```

**Alexa+ features:**
- Conversational AI with persistent context
- Multi-turn conversations
- Complex reasoning and creative tasks
- Source citations when applicable

> **Note:** Requires Alexa+ to be enabled on your Amazon account.

### Activation Phrase Poller

`alexa-poller` is a companion binary for route-based voice automation. It listens for activation phrases in Alexa activity and dispatches the remaining text to a destination such as OpenClaw, a shell command, or stdout.

Use it when you want to say something like "clawtto summarize my unread emails" to an Echo device and have the payload `summarize my unread emails` routed to an automation.

#### Quick Start

```bash
# Authenticate first. alexa-poller uses the same config as alexacli.
alexacli auth

# Route "clawtto ..." from one Echo device into OpenClaw's main agent.
alexa-poller route add clawtto --device "Echo Show" --to openclaw:main --source auto

# Review configured routes.
alexa-poller route list

# Start watching Alexa activity.
alexa-poller run
```

If you say `clawtto summarize my unread emails`, the route dispatches `summarize my unread emails`.

#### Route Management

```bash
# Add a route.
alexa-poller route add clawtto --device "Echo Show" --to openclaw:main

# Add a multi-word activation phrase. Quote it so it is passed as one argument.
alexa-poller route add "brief me" --to 'exec:/Users/buddy/bin/dispatch "{{text}}"'

# Print matched payloads instead of dispatching them.
alexa-poller route add house --to stdout

# List routes for humans.
alexa-poller route list

# List routes as JSON for scripts.
alexa-poller route list --json

# Remove a route by ID from route list output.
alexa-poller route remove route-abc123ef
```

Routes are stored in `~/.alexa-cli/poller.json`.

Example config:

```json
{
  "routes": [
    {
      "id": "route-abc123ef",
      "phrase": "clawtto",
      "device": "Echo Show",
      "source": "auto",
      "match": "prefix",
      "ignore": ["nevermind", "ignore this"],
      "action": {
        "type": "openclaw",
        "agent": "main"
      },
      "ack": {
        "text": "Sent"
      }
    }
  ]
}
```

#### Sources

`--source` controls where the poller looks for activation phrases:

| Source | Behavior |
|--------|----------|
| `auto` | Default. Checks Alexa+ conversation fragments first, then voice history. |
| `conversation` | Only checks Alexa+ conversation fragments. Requires Alexa+ conversations for the account/device. |
| `history` | Only checks standard Alexa voice activity history. Best compatibility fallback. |

Use `auto` unless you know you need one backend. If Alexa+ is not enabled or fragments are unavailable, `auto` still attempts history polling.

#### Match Modes

`--match` controls how the activation phrase is found:

| Match | Example input | Payload |
|-------|---------------|---------|
| `prefix` | `clawtto summarize my unread emails` | `summarize my unread emails` |
| `contains` | `Alexa, brief me on today` with phrase `brief me` | `on today` |

`prefix` is the default and is usually safer because ordinary speech is less likely to accidentally trigger a route. `contains` is useful when Alexa transcription tends to add words before your phrase.

The payload must not be empty. Saying only `clawtto` will not dispatch.

#### Destinations

`--to` selects what receives the matched payload:

| Destination | Format | Behavior |
|-------------|--------|----------|
| OpenClaw default agent | `openclaw` | Runs `openclaw agent --message <payload> --agent main`. |
| OpenClaw named agent | `openclaw:<agent>` | Runs `openclaw agent --message <payload> --agent <agent>`. |
| Shell command | `exec:<command>` | Runs the command through `/bin/sh -c`; `{{text}}` is replaced with the payload. |
| Stdout | `stdout` | Prints the timestamp, route phrase, and payload. Useful for testing. |

Examples:

```bash
# Send to OpenClaw's main agent.
alexa-poller route add clawtto --to openclaw:main

# Send to a named OpenClaw agent.
alexa-poller route add research --to openclaw:research

# Run a local script. Quote the destination so the shell preserves {{text}}.
alexa-poller route add "brief me" --to 'exec:/Users/buddy/bin/dispatch "{{text}}"'

# Test matching without side effects.
alexa-poller route add house --to stdout
```

For `exec:` destinations, treat the payload as untrusted user input. Prefer passing `{{text}}` as a quoted argument to a script you control instead of interpolating it into a complex shell pipeline.

#### Device Filtering

Use `--device` to limit a route to one Echo device:

```bash
alexa-poller route add clawtto --device "Kitchen Echo" --to stdout
```

The device name must match the Alexa activity source device name. Use `alexacli devices` and `alexacli conversations` to inspect available names. If `--device` is omitted, any device can trigger the route.

#### Ignore Phrases and Acknowledgements

The poller always ignores activation attempts that start with:

- `nevermind`
- `ignore this`

Add more ignore phrases with `--ignore`:

```bash
alexa-poller route add clawtto --ignore "cancel that" --ignore "scratch that"
```

Use `--ack` to have Alexa speak a short acknowledgement after a route dispatches:

```bash
alexa-poller route add clawtto --device "Echo Show" --to openclaw:main --ack "Sent"
```

If `--device` is set, the acknowledgement is spoken on that device. Otherwise, the poller tries to speak on the originating device from the matched event.

#### Running the Poller

```bash
alexa-poller run
```

On startup, the poller loads devices, checks configured sources, and seeds its dedupe state with existing records. This prevents old Alexa history or conversation fragments from firing as soon as the process starts. After the initial seed, only new matching events dispatch.

The poller stores dedupe state in `~/.alexa-cli/poller-state.json`, so it can restart without replaying already-seen records.

By default the poller checks every 5 seconds. Advanced users can set `pollInterval` in `~/.alexa-cli/poller.json`:

```json
{
  "pollInterval": "2s",
  "routes": []
}
```

Run it under your preferred process supervisor for long-lived automation, such as `launchd`, `systemd`, `tmux`, or a user-level service wrapper.

#### End-to-End Test Route

Use `stdout` first when validating a new account, device, or activation phrase:

```bash
alexa-poller route add poller-test --device "Echo Show" --source history --to stdout
alexa-poller run
```

Say `poller-test hello from the kitchen`. Expected output:

```text
[2026-06-26T15:00:00-04:00] poller-test -> hello from the kitchen
```

After that works, change `--to` to your real destination.

#### Poller Files

| Path | Purpose |
|------|---------|
| `~/.alexa-cli/config.json` | Shared `alexacli` auth/config. |
| `~/.alexa-cli/poller.json` | Poller routes and optional poll interval. |
| `~/.alexa-cli/poller-state.json` | Seen history/fragments used for dedupe. |

Delete `poller-state.json` only if you intentionally want the poller to reseed from current Alexa activity on next startup.

### Audio Playback

Play MP3 audio through Alexa devices using SSML:

```bash
# Play audio from HTTPS URL
alexacli play --url "https://example.com/audio.mp3" -d "Echo Show"
```

Requirements:
- MP3 format: 48kbps bitrate, 22050Hz sample rate
- HTTPS URL with valid SSL certificate
- Convert audio: `ffmpeg -i input.mp3 -ar 22050 -ab 48k -ac 1 output.mp3`

### Routines (Coming Soon)

```bash
# List available routines
alexacli routine list

# Execute a routine
alexacli routine run "Good Night"
```

### Direct Smart Home API (Coming Soon)

For granular, programmatic control of smart home devices without natural language:

```bash
# List smart home devices with IDs and capabilities
alexacli sh list

# Direct device control by name
alexacli sh on "Kitchen Light"
alexacli sh off "All Lights"
alexacli sh brightness "Bedroom Lamp" 50
```

> **Note:** For most use cases, especially AI agents, `alexacli command` is recommended. Natural language commands are more flexible and match how you'd interact with Alexa verbally. The direct API is useful when you need exact device IDs or want to avoid natural language parsing.

## JSON Output

All commands support `--json` for machine-readable output:

```bash
alexacli devices --json | jq '.[].name'
alexacli speak "test" -d Kitchen --json
```

## Command Reference

| Command | Description | Status |
|---------|-------------|--------|
| `alexacli devices` | List all Echo devices | Working |
| `alexacli speak <text> -d <device>` | Text-to-speech on device | Working |
| `alexacli speak <text> --announce` | Announce to all devices | Working |
| `alexacli command <text> -d <device>` | Voice command (smart home, music, etc.) | Working |
| `alexacli ask <text> -d <device>` | Send command, get response back | Working |
| `alexacli history` | View recent voice activity | Working |
| `alexacli conversations` | List Alexa+ conversation IDs | Working |
| `alexacli fragments <id>` | View Alexa+ conversation history | Working |
| `alexacli askplus -c <id> <text>` | Send message to Alexa+ LLM | Working |
| `alexacli play --url <url> -d <device>` | Play MP3 audio via SSML | Working |
| `alexacli auth` | Browser login or manual token setup | Working |
| `alexacli auth status` | Show auth status (with `--verify`) | Working |
| `alexacli auth logout` | Remove stored credentials | Working |
| `alexa-poller route add <phrase>` | Add an activation phrase route | Working |
| `alexa-poller route list` | List configured poller routes | Working |
| `alexa-poller route list --json` | List poller routes as JSON | Working |
| `alexa-poller route remove <id>` | Remove a poller route | Working |
| `alexa-poller run` | Watch Alexa activity and dispatch route matches | Working |
| `alexacli routine list` | List routines | WIP |
| `alexacli routine run <name>` | Execute routine | WIP |
| `alexacli sh list` | List smart home devices | WIP |
| `alexacli sh on/off <device>` | Control device | WIP |

> **Note:** Routines and Direct Smart Home API are work-in-progress. Use `alexacli command` for smart home control - it's fully working and actually preferred for AI/agentic use since natural language is more flexible than device IDs.

## Use Cases

### Claude Code / AI Agent Integration

This CLI was built specifically to allow AI assistants to control smart home devices. The natural language `command` interface is ideal for agentic use - the AI can construct commands the same way a human would speak to Alexa:

```bash
# AI can control any smart home device using natural language
alexacli command "turn off all the lights" -d Kitchen
alexacli command "set the thermostat to 68" -d Kitchen
alexacli command "lock the front door" -d Kitchen

# Notifications and announcements
alexacli speak "The build finished successfully" -d Office
alexacli speak "Reminder: standup in 5 minutes" --announce
```

### Scripting and Automation

```bash
#!/bin/bash
# Announce when a long-running job finishes
make build && alexacli speak "Build complete!" --announce
```

### Home Automation

```bash
# Morning routine script
alexacli command "good morning" -d Bedroom
alexacli command "turn on kitchen lights" -d Kitchen
alexacli command "what's on my calendar today" -d Kitchen
```

## Token Refresh

The refresh token is valid for approximately 14 days. If you get authentication errors, run `alexacli auth` again to re-authenticate.

## Troubleshooting

### Debug mode

Add `--verbose` or `-v` to any command to see API requests and responses:

```bash
alexacli devices -v
alexacli command "turn on lights" -d Kitchen --verbose
```

### "not configured" error

Run `alexacli auth` to configure your refresh token.

For EU/IT accounts, prefer:

```bash
alexacli auth --domain amazon.com --country amazon.it
```

### Device not found

Use `alexacli devices` to see exact device names, then match them in your commands. Partial matching is supported.

### Command not working

Try running the same command with `alexacli command` instead - this sends it as a voice command which has broader support.

### Poller route not firing

Start with a stdout route and the history source:

```bash
alexa-poller route add poller-test --device "Echo Show" --source history --to stdout
alexa-poller run
```

Then say `poller-test hello`. If nothing prints:

- Run `alexacli auth status --verify` to confirm credentials still work.
- Run `alexacli history --limit 10` to confirm Alexa recorded the utterance.
- Confirm the route device name matches the device shown in Alexa activity.
- Try removing `--device` temporarily to rule out a device-name mismatch.
- Try `--match contains` if Alexa adds words before your phrase.
- Wait for a new utterance after starting the poller; startup seeds existing records and does not replay old history.

### Poller dispatch failed

For `openclaw` routes, make sure `openclaw` is installed and available on the `PATH` of the process running `alexa-poller`.

For `exec:` routes:

- Quote the destination so your shell does not expand `{{text}}`.
- Prefer `--to 'exec:/path/to/script "{{text}}"'`.
- Run the same command manually with sample text.
- Check file permissions on the target script.

### Poller repeats old events

The poller dedupes records through `~/.alexa-cli/poller-state.json`. If that file is deleted, moved, or not writable, the poller will seed again on startup. It should not dispatch old events during the seed pass, but it may need one poll cycle before new events are eligible.

### Poller sees no Alexa+ conversations

Use `alexacli conversations` to confirm Alexa+ conversations are available for the account. If they are not available, use `--source history` or leave the route on the default `--source auto` so the poller can fall back to voice history.

## How It Works

This CLI uses the same unofficial API that the Alexa mobile app uses. It:

1. Exchanges your refresh token for session cookies via Amazon auth (`api.amazon.com` with your `amazon_domain`)
2. Obtains CSRF tokens from Alexa web endpoints (`alexa.*`, with fallback hosts)
3. Sends command APIs to regional runtime endpoints:
   - US/CA/BR/IN: `pitangui.*`
   - EU (including IT): `layla.amazon.com`
4. For history/privacy APIs, uses `www.<amazon_local>` with fallback to auth/global domains when needed (to handle temporary marketplace-side blocks)
5. `alexa-poller` reuses the same auth config, polls Alexa+ conversations and/or voice history, dedupes seen records, matches configured activation phrases, and dispatches the matched payload to the route destination.

This approach is used by many popular projects including [alexa-remote-control](https://github.com/thorsten-gehrig/alexa-remote-control) and [Home Assistant's Alexa integration](https://github.com/alandtse/alexa_media_player).

## Disclaimer

This is an unofficial tool that uses Amazon's private APIs. It may break at any time if Amazon changes their API. Use at your own risk.

## Acknowledgments

This project builds on the work of several excellent open source projects:

- **[alexa-cookie-cli](https://github.com/adn77/alexa-cookie-cli)** by adn77 - Token authentication tool (required for setup)
- **[alexa-cookie2](https://github.com/Apollon77/alexa-cookie2)** by Apollon77 - The underlying authentication library
- **[alexa-remote-control](https://github.com/thorsten-gehrig/alexa-remote-control)** by thorsten-gehrig - Bash implementation that documented the API
- **[alexa_media_player](https://github.com/alandtse/alexa_media_player)** by alandtse - Home Assistant integration that proved the approach at scale

Without these projects' reverse-engineering efforts and documentation, this CLI wouldn't exist.

## License

MIT
