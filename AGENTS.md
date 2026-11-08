# AGENTS.md

## 1. Ringkasan proyek

One More Percent adalah bot Telegram pribadi (satu pengguna) berbasis AI. Bot
mengirim pengingat sesuai jadwal, mencatat progres dari percakapan natural,
mengirim rekap harian, dan bisa membuat/mengubah/menghapus jadwal lewat chat.
Database-nya adalah Google Sheets; tidak ada PostgreSQL.

## 2. Tech stack

- Go (standard library saja)
- Google Sheets REST API + service account (JWT ditandatangani sendiri)
- Telegram Bot API via webhook
- AI: Groq atau Google AI Studio (Gemini), dipilih lewat `AI_PROVIDER`
- Docker / Docker Compose / Render (free web service)

## 3. Peta folder

```
cmd/bot/            entrypoint bot: load config, wire dependency, run
cmd/setup/          buat worksheet + header, seed jadwal contoh
internal/config/    load & validasi environment + .env
internal/domain/    struct dan interface repository/messenger (tanpa import internal lain)
internal/repository/sheets/  implementasi Google Sheets, cache, retry, seed
internal/ai/        interface Client + helper HTTP
internal/ai/groq/   provider Groq
internal/ai/gemini/ provider Gemini
internal/ai/factory/ memilih provider dari config
internal/prompt/    loader template, hot reload, embed fallback
internal/service/   logika bisnis: chat, CRUD jadwal, progres, reminder, recap
internal/telegram/  client Bot API + tipe Update/Message
internal/scheduler/ ticker pengingat & rekap
internal/httpserver/ router, webhook, healthz
prompts/            prompt AI yang bisa diedit user
docs/               panduan setup
```

Arah dependensi: `httpserver`/`scheduler` → `service` → `domain` ←
`repository`/`ai`/`telegram`. `domain` tidak mengimpor package internal lain.

## 4. Perintah penting

```bash
make run            # go run ./cmd/bot
make test           # go test ./...
make vet            # go vet ./...
make lint           # alias go vet
make build          # go build ./...
make setup-sheet    # buat worksheet + seed contoh
docker compose up --build -d
```

## 5. Aturan kode

- Jangan hardcode URL API, ID, nama model, sheet, jam rekap, timezone, atau
  secret. Semua lewat `internal/config` dan `.env`.
- Menambah env baru wajib memperbarui `.env.example`, `internal/config`, dan
  README.
- Akses data hanya lewat interface di `domain`. Akses AI hanya lewat
  `ai.Client` (jangan impor `ai/groq` atau `ai/gemini` dari service).
- Semua panggilan eksternal memakai `context` + timeout.
- Bungkus error dengan konteks: `fmt.Errorf("...: %w", err)`.
- Tidak ada variabel global untuk state. Inject lewat constructor.
- Logging memakai `log/slog`; jangan pernah mencetak token/API key/credentials.
- Jangan menambah komentar kecuali diperlukan.

## 6. Aturan prompt

- Teks prompt hanya boleh ada di `prompts/`. Jangan menaruh string prompt di
  file `.go`.
- Jika menambah variabel template, perbarui `prompts/README.md` dan service
  yang merender prompt tersebut.
- Prompt default untuk fallback ada di `internal/prompt/defaults/`.

## 7. Menambah AI provider baru

1. Buat package `internal/ai/<nama>` yang mengimplementasikan `ai.Client`.
2. Jangan impor package provider lain; hanya `internal/ai` dan stdlib.
3. Daftarkan di `internal/ai/factory/factory.go`.
4. Tambah env (`<NAMA>_API_KEY`, `<NAMA>_BASE_URL`, `<NAMA>_MODEL`) ke
   `.env.example`, `internal/config`, dan README.

## 8. Menambah worksheet/tabel baru

1. Tambah struct + interface di `internal/domain`.
2. Implementasi di `internal/repository/sheets` (pakai `Client` yang ada).
3. Tambah header di `internal/repository/sheets/schema.go` dan daftarkan di
   `EnsureSheets` (`internal/repository/sheets/client.go`).
4. Dokumentasikan nama sheet + kolom di README/docs.

## 9. Testing

- Unit test service memakai fake repository dan fake `ai.Client`.
- Jangan memanggil Google Sheets, Telegram, Groq, atau Gemini sungguhan di test.
- Jalankan `go build ./...`, `go vet ./...`, `go test ./...` sebelum selesai.

## 10. Keamanan

- Jangan commit `.env`, file credentials Google, atau folder `tmp/`.
- Webhook wajib divalidasi dengan header `X-Telegram-Bot-Api-Secret-Token`.
- Hanya `TELEGRAM_USER_ID` yang boleh memakai bot.
- Untuk hosting tanpa file secret, pakai `GOOGLE_CREDENTIALS_JSON`.

## 11. Definition of done

- `go build ./...`, `go vet ./...`, `go test ./...` lulus.
- `.env.example`, README, dan docs ikut diperbarui.
- Tidak ada URL/ID/model hardcode baru di `internal/` atau `cmd/`.
- Prompt tetap berada di `prompts/`.
