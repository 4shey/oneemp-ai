# One More Percent

One More Percent is a personal Telegram accountability bot. It sends schedule
reminders, tracks progress from natural conversation, sends a daily recap, and
lets the user manage schedules through chat. Data is stored in Google Sheets, so
there is no separate database server.

The bot persona is **Oneemp**: a direct, human-sounding accountability partner.
It is usually calm and concise, but it becomes strict when a schedule is close or
the user keeps ignoring it.

## Features

- Schedule reminders for one-time, daily, and weekly schedules.
- Overnight schedules, such as sleep from 23:30 to 04:30.
- Start reminders and end/wake-up reminders.
- Natural-language schedule creation, updates, deletion, and progress logging.
- Reads the user's schedule context before replying, so it can distinguish
  "about to start" from "already finished".
- Daily recap with completed and missed items.
- `/sudo` mode for database writes and full-table reads.
- Google Sheets storage with reminder idempotency and progress archiving.
- Editable AI prompts in `prompts/`.
- Gemini and Groq providers.

## How it works

```text
Telegram
  -> POST /webhook
  -> validate webhook secret and allowed Telegram user
  -> AI intent extraction
  -> schedule/progress operation or normal chat
  -> Google Sheets
  -> Telegram reply
```

The scheduler runs inside the bot process. It checks reminders on a ticker,
sends due reminders, and creates the daily recap.

## Tech stack

| Component | Stack |
| --- | --- |
| Language | Go standard library |
| Storage | Google Sheets REST API with a service account |
| AI | Google AI Studio (Gemini) or Groq |
| Messaging | Telegram Bot API webhook |
| Runtime | Docker / Docker Compose |

## Local setup with Docker

### 1. Prerequisites

- Docker Desktop with Docker Compose
- A Telegram bot token from BotFather
- Your numeric Telegram user ID
- A Google Cloud service account with Google Sheets API enabled
- A Google Sheets spreadsheet shared with the service account
- A Gemini or Groq API key
- `cloudflared` or another HTTPS tunnel for local webhook testing

### 2. Clone the repository

```bash
git clone <repository-url>
cd ONEMOREPERCENT
```

### 3. Create the environment file

```bash
cp .env.example .env
```

Fill the required values in `.env`:

```env
PUBLIC_BASE_URL=https://your-public-url.example.com
TELEGRAM_BOT_TOKEN=123456:replace-me
TELEGRAM_USER_ID=123456789
TELEGRAM_WEBHOOK_SECRET=replace-with-a-random-string
TELEGRAM_API_BASE_URL=https://api.telegram.org

SHEETS_SPREADSHEET_ID=replace-with-spreadsheet-id
GOOGLE_CREDENTIALS_JSON={"type":"service_account",...}
GOOGLE_SCOPE=https://www.googleapis.com/auth/spreadsheets
SHEETS_API_BASE_URL=https://sheets.googleapis.com

AI_PROVIDER=gemini
GEMINI_API_KEY=replace-me
GEMINI_BASE_URL=https://generativelanguage.googleapis.com/v1beta
GEMINI_MODEL=gemini-3.1-flash-lite

PROMPTS_DIR=./prompts
PROMPTS_HOT_RELOAD=true
```

For Docker, `GOOGLE_CREDENTIALS_JSON` is the easiest option: paste the service
account JSON as one line. Docker Compose sets `PROMPTS_DIR` to `/app/prompts`
automatically.

`USER_CONTEXT` is optional. Use it for personal context that the bot may use when
the user asks about goals or when a schedule is missed.

### 4. Create the worksheets

```bash
docker compose run --rm --entrypoint ./setup app
```

This creates the required worksheets, headers, and example schedules.

### 5. Start the bot

```bash
docker compose up --build -d
```

Check the logs:

```bash
docker compose logs -f app
```

### 6. Expose the webhook locally

Telegram needs a public HTTPS URL. For local development, use a tunnel:

```bash
cloudflared tunnel --url http://localhost:8080
```

Copy the generated HTTPS URL into `PUBLIC_BASE_URL` in `.env`, then recreate the
container so the webhook is registered again:

```bash
docker compose up -d --force-recreate
```

The tunnel URL changes when the tunnel restarts. Update `PUBLIC_BASE_URL` and
recreate the container whenever that happens.

### 7. Use the bot

Open your bot in Telegram and send a message. Only the Telegram user ID in
`TELEGRAM_USER_ID` is allowed to use it.

Stop the stack with:

```bash
docker compose down
```

## Usage

### Read-only chat

These do not need `/sudo`:

- `list jadwal`
- `jadwal hari ini`
- `progress hari ini`
- normal conversation

### `/sudo` commands

`/sudo` must be the first token, exactly lowercase, with no leading space.
Database writes and admin table reads require it.

| Table | Operations |
| --- | --- |
| `schedules` | create, update, delete (list does not require `/sudo`) |
| `progress_logs` | complete, delete, list |
| `progress_archive` | upsert, delete, list |
| `reminder_state` | upsert, delete, list |

Examples:

- `/sudo buat jadwal olahraga besok jam 7`
- `/sudo ubah jadwal olahraga jadi jam 8`
- `/sudo hapus jadwal olahraga`
- `/sudo udah selesai baca buku 30 menit`
- `/sudo ga sempat olahraga`
- `/sudo hapus progres baca buku`
- `/sudo list progress`
- `/sudo list archive`
- `/sudo list reminders`
- `/sudo tambah arsip baca buku tanggal 2026-11-01 status selesai`
- `/sudo hapus arsip baca buku`
- `/sudo hapus reminder sebelum 2026-01-01`
- `/sudo list jadwal`
- `/sudo setuju` (confirm a pending proposal)

Without `/sudo`, the bot will not change data. It will ask the user to retry with
`/sudo`. `/sudo` in the middle of a message, or `/sudo` with no command, is
rejected.

Confirmation is also a database action. If the bot asks for confirmation, reply
with `/sudo setuju` or `/sudo ya`. A reply without `/sudo` discards the pending
proposal and does not execute it.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `PUBLIC_BASE_URL` | Public HTTPS URL used for the Telegram webhook |
| `TELEGRAM_BOT_TOKEN` | BotFather token |
| `TELEGRAM_USER_ID` | The only Telegram user allowed to use the bot |
| `TELEGRAM_WEBHOOK_SECRET` | Secret validated on every webhook request |
| `TELEGRAM_API_BASE_URL` | Telegram Bot API base URL |
| `SHEETS_SPREADSHEET_ID` | Google Sheets spreadsheet ID |
| `GOOGLE_CREDENTIALS_JSON` | Service account JSON on one line |
| `GOOGLE_CREDENTIALS_FILE` | Alternative to `GOOGLE_CREDENTIALS_JSON` |
| `GOOGLE_SCOPE` | Google Sheets scope |
| `SHEETS_API_BASE_URL` | Google Sheets API base URL |
| `AI_PROVIDER` | `gemini` or `groq` |
| `GEMINI_API_KEY` | Gemini API key |
| `GROQ_API_KEY` | Groq API key |
| `USER_CONTEXT` | Optional personal context for reminders and reflection |
| `PROMPTS_DIR` | Prompt directory, default `./prompts` |
| `PROMPTS_HOT_RELOAD` | Reload prompt files without restarting |

See `.env.example` for the full list and defaults.

## Project structure

```text
cmd/
  bot/          bot entrypoint
  setup/        worksheet setup and seed data
internal/
  ai/           AI client, providers, and factory
  config/       environment loading and validation
  domain/       domain structs and repository interfaces
  httpserver/   webhook, health check, typing indicator
  prompt/       prompt loader and embedded defaults
  repository/   Google Sheets repositories
  scheduler/    reminder and recap ticker
  service/      chat, schedule, progress, and reminder logic
  telegram/     Telegram Bot API client
prompts/        editable AI prompts
docs/           setup and deployment notes
```

## Prompts

All prompt templates are in `prompts/`. The loader reads them at runtime when
`PROMPTS_HOT_RELOAD=true`, so prompt changes can be applied without rebuilding
the Go binary. Keep the copies in `internal/prompt/defaults/` in sync so the
embedded fallback matches.

See [prompts/README.md](prompts/README.md) for variables and behavior rules.

## Security

- Never commit `.env` or Google service account credentials.
- The webhook validates `X-Telegram-Bot-Api-Secret-Token`.
- Only `TELEGRAM_USER_ID` can use the bot.
- Database writes and admin reads require `/sudo`.
- `USER_CONTEXT` is stored in the environment, not in the repository.
