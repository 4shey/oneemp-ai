# Tutorial Deploy One More Percent ke Render

Panduan ini menjelaskan cara hosting bot di **Render free** supaya tetap hidup
24 jam. Bot berjalan sebagai Docker web service, memakai Google Sheets sebagai
database, Telegram webhook, dan Gemini sebagai AI default.

## 0. Prasyarat

Siapkan sebelum mulai:

- Akun GitHub dan repo proyek ini sudah di-push.
- Akun Render: https://render.com
- Bot Telegram dari **@BotFather**.
- Google Cloud project dengan **Google Sheets API** aktif dan service account.
- API key Gemini dari Google AI Studio.
- Spreadsheet Google yang sudah di-share ke `client_email` service account.
- Docker Desktop (untuk setup sheet dari lokal).
- Git dan PowerShell.

Alur besarnya:

```
GitHub repo -> Render (Docker) -> Telegram webhook
                     |
                     +-> Google Sheets
                     +-> Gemini API
```

## 1. Siapkan Google Sheets

Ikuti [google-sheets-setup.md](google-sheets-setup.md). Ringkasnya:

1. Buat project di Google Cloud.
2. Aktifkan **Google Sheets API**.
3. Buat **service account**, lalu buat key JSON.
4. Buat spreadsheet baru.
5. Share spreadsheet ke `client_email` service account sebagai **Editor**.
6. Simpan:
   - `SHEETS_SPREADSHEET_ID` dari URL spreadsheet.
   - File JSON service account (misal `google-credentials.json`).

Worksheet tidak perlu dibuat manual. `cmd/setup` atau startup bot akan membuat
sheet dan header otomatis.

## 2. Siapkan Telegram

1. Chat **@BotFather**, buat bot baru, simpan token:
   ```
   TELEGRAM_BOT_TOKEN=123456:ABC...
   ```
2. Buka bot Anda, tekan **Start**. Ini wajib agar Telegram mengizinkan bot
   mengirim pesan.
3. Dapatkan ID numerik Anda dari **@userinfobot** atau bot sejenis:
   ```
   TELEGRAM_USER_ID=123456789
   ```
4. Buat secret acak untuk webhook:
   ```powershell
   [guid]::NewGuid().ToString('N')
   ```
   Simpan sebagai `TELEGRAM_WEBHOOK_SECRET`.

Jangan memakai username Telegram. Bot membutuhkan ID numerik.

## 3. Siapkan AI (Gemini)

Default proyek: `AI_PROVIDER=gemini`.

Ambil API key dari Google AI Studio, lalu isi:

```env
AI_PROVIDER=gemini
GEMINI_API_KEY=...
GEMINI_BASE_URL=https://generativelanguage.googleapis.com/v1beta
GEMINI_MODEL=gemini-3.1-flash-lite
```

Nama model Gemini bisa berubah. Cek daftar model yang tersedia untuk key Anda:

```powershell
$key="GEMINI_API_KEY_ANDA"
Invoke-RestMethod "https://generativelanguage.googleapis.com/v1beta/models?key=$key" |
  Select-Object -ExpandProperty models |
  Where-Object { $_.supportedGenerationMethods -contains 'generateContent' } |
  Select-Object -ExpandProperty name
```

Alternatif Groq ada di [ai-providers.md](ai-providers.md).

## 4. Siapkan kredensial Google untuk Render

Render free **tidak punya persistent disk**, jadi jangan memakai
`GOOGLE_CREDENTIALS_FILE` di Render. Gunakan `GOOGLE_CREDENTIALS_JSON`.

Buat JSON satu baris dari file service account:

```powershell
$one = (Get-Content .\google-credentials.json -Raw | ConvertFrom-Json) |
  ConvertTo-Json -Compress -Depth 50
$one | Set-Clipboard
```

Tempel hasilnya ke environment variable `GOOGLE_CREDENTIALS_JSON` di Render.
Walaupun JSON multi-baris bisa diterima environment variable, versi satu baris
lebih aman untuk copy/paste dan YAML.

## 5. Setup worksheet dari lokal

Lakukan sekali dari komputer Anda. Ini membuat sheet + seed 56 jadwal contoh.

1. Salin env:
   ```powershell
   Copy-Item .env.example .env
   ```
2. Isi `.env` lokal:
   ```env
   PUBLIC_BASE_URL=https://placeholder.example.com
   TELEGRAM_BOT_TOKEN=...
   TELEGRAM_USER_ID=...
   TELEGRAM_WEBHOOK_SECRET=...
   SHEETS_SPREADSHEET_ID=...
   GOOGLE_CREDENTIALS_FILE=./google-credentials.json
   AI_PROVIDER=gemini
   GEMINI_API_KEY=...
   ```
   `PUBLIC_BASE_URL` wajib diisi karena config divalidasi, walaupun untuk
   `cmd/setup` nilainya tidak dipakai untuk webhook.
3. Jalankan setup:
   ```powershell
   docker compose run --rm --entrypoint ./setup app
   ```
   Atau jika Go terpasang:
   ```powershell
   go run ./cmd/setup
   ```
4. Pastikan output:
   ```
   Worksheet dan header siap dipakai.
   Seed selesai: 56 jadwal ditambahkan.
   ```

Jika spreadsheet sudah berisi jadwal, seed akan dilewati.

## 6. Deploy ke Render

### Cara A — Blueprint `render.yaml` (disarankan)

1. Push repo ke GitHub.
2. Buka Render Dashboard -> **New** -> **Blueprint**.
3. Pilih repo.
4. Render membaca `render.yaml` dan membuat web service Docker.
5. Isi environment variable yang ditandai `sync: false` di dashboard.
6. Klik **Deploy**.

`render.yaml` sudah mengatur:

- runtime Docker
- plan free
- health check `/healthz`
- auto deploy setiap push
- default `AI_PROVIDER=gemini` dan `GEMINI_MODEL=gemini-3.1-flash-lite`

### Cara B — Manual

1. **New** -> **Web Service** -> pilih repo.
2. Runtime: **Docker**.
3. Plan: **Free**.
4. Health check path: `/healthz`.
5. Jangan set `APP_PORT`. Render otomatis memberi env `PORT`.
6. Tambahkan env yang diperlukan (lihat tabel di bawah).
7. Deploy.

## 7. Environment variable Render

Wajib:

| Key | Nilai |
| --- | --- |
| `PUBLIC_BASE_URL` | `https://<nama-service>.onrender.com` |
| `TELEGRAM_BOT_TOKEN` | token dari BotFather |
| `TELEGRAM_USER_ID` | ID numerik Telegram Anda |
| `TELEGRAM_WEBHOOK_SECRET` | string acak, sama dengan yang dipakai bot |
| `SHEETS_SPREADSHEET_ID` | ID spreadsheet |
| `GOOGLE_CREDENTIALS_JSON` | isi file JSON service account, satu baris |
| `GOOGLE_SCOPE` | `https://www.googleapis.com/auth/spreadsheets` |
| `AI_PROVIDER` | `gemini` |
| `GEMINI_API_KEY` | API key Gemini |
| `GEMINI_BASE_URL` | `https://generativelanguage.googleapis.com/v1beta` |
| `GEMINI_MODEL` | `gemini-3.1-flash-lite` |

Opsional (default sudah aman):

| Key | Default |
| --- | --- |
| `APP_TIMEZONE` | `Asia/Jakarta` |
| `WEBHOOK_PATH` | `/webhook` |
| `CRON_TICK_SECONDS` | `60` |
| `DAILY_RECAP_TIME` | `00:00` |
| `LOG_LEVEL` | `info` |
| `SHUTDOWN_TIMEOUT_SECONDS` | `15` |
| `SHEETS_CACHE_TTL_SECONDS` | `60` |
| `SHEET_AUTO_CREATE` | `true` |
| `SHEET_ARCHIVE_ENABLED` | `true` |
| `PROGRESS_RETENTION_DAYS` | `90` |
| `REMINDER_RETENTION_DAYS` | `7` |
| `AI_TIMEOUT_SECONDS` | `30` |
| `AI_TEMPERATURE` | `0.3` |
| `AI_JSON_RETRIES` | `2` |
| `PROMPTS_DIR` | `/app/prompts` |
| `PROMPTS_HOT_RELOAD` | `true` |
| `USER_CONTEXT` | kosong |

Catatan:

- Isi `USER_CONTEXT` dengan alasan/impian user. Nilainya hanya disimpan sebagai
  environment variable dan tidak masuk repo.

- Jangan mengisi `APP_PORT` di Render.
- `PUBLIC_BASE_URL` harus HTTPS dan tanpa slash di akhir.
- Setelah mengganti `PUBLIC_BASE_URL`, redeploy supaya webhook didaftarkan ulang.

## 8. Setelah deploy

Cek log Render. Harusnya ada:

```
Worksheet dan header siap dipakai.
webhook terdaftar
scheduler berjalan
server berjalan
```

Cek health:

```powershell
Invoke-RestMethod https://<nama-service>.onrender.com/healthz
```

Cek webhook Telegram:

```powershell
$token="TELEGRAM_BOT_TOKEN_ANDA"
Invoke-RestMethod "https://api.telegram.org/bot$token/getWebhookInfo"
```

Pastikan:

- `url` = `https://<nama-service>.onrender.com/webhook`
- `pending_update_count` kecil/0
- `last_error_message` kosong

Lalu kirim pesan ke bot. Jika tidak dibalas, lihat bagian troubleshooting.

## 9. Keep-alive agar tidak tidur

Render free bisa tidur setelah ±15 menit tanpa trafik. Karena scheduler
pengingat ada di dalam proses web, service harus tetap bangun.

Repo ini punya workflow `.github/workflows/keepalive.yml` yang memanggil
`/healthz` tiap 5 menit.

1. Buka GitHub repo -> **Settings** -> **Secrets and variables** -> **Actions**.
2. Buat secret:
   ```
   HEALTHCHECK_URL=https://<nama-service>.onrender.com/healthz
   ```
3. Buka tab **Actions**, aktifkan workflow **keep-alive**.

Catatan:

- GitHub Actions bisa menunda jadwal beberapa menit.
- Workflow scheduled bisa nonaktif jika repo lama tidak ada aktivitas.
- Alternatif yang lebih andal: buat cron gratis di https://cron-job.org yang
  memanggil `HEALTHCHECK_URL` tiap 5 menit.
- Render free memberi 750 jam instance per bulan. Satu service yang hidup terus
  memakai sekitar 730 jam.

## 10. Update aplikasi

- Perubahan kode: push ke GitHub. Render auto-deploy jika `autoDeploy: true`.
- Perubahan env: edit di dashboard Render, lalu **Manual Deploy**.
- Perubahan prompt: `prompts/` di-copy ke image saat build, jadi harus
  redeploy. Hot reload hanya berguna jika folder di-mount (lokal).
- Setelah mengubah prompt, salin juga ke `internal/prompt/defaults/`.

## 11. Troubleshooting Render

| Gejala | Penyebab | Solusi |
| --- | --- | --- |
| Deploy gagal saat build | Dockerfile/Go error | cek log build Render, jalankan `docker compose build` lokal |
| App exit: `PUBLIC_BASE_URL wajib diisi` | env kosong | isi URL Render |
| App exit: kredensial Google | JSON salah / tidak diisi | isi `GOOGLE_CREDENTIALS_JSON` satu baris |
| `webhook terdaftar` tidak muncul | URL salah / koneksi | cek `PUBLIC_BASE_URL` dan log |
| Bot tidak membalas | webhook salah/lokal tidur | cek `getWebhookInfo`, `pending_update_count`, `last_error_message` |
| `chat not found` | user belum Start bot | buka bot, tekan Start |
| Google 403 | sheet belum di-share | share ke `client_email` sebagai Editor |
| Google 403 `API not enabled` | Sheets API belum aktif | aktifkan di Google Cloud |
| AI 404 model | model Gemini/Groq tidak tersedia | ganti `GEMINI_MODEL`/`GROQ_MODEL` |
| AI 429 | kuota/rate limit | tunggu, kurangi frekuensi, ganti key |
| Service tidur | keep-alive tidak jalan | aktifkan workflow/cron eksternal |
| Prompt tidak berubah | image lama | redeploy |
| Spreadsheet penuh | retensi | cek `SHEET_ARCHIVE_ENABLED`, `PROGRESS_RETENTION_DAYS` |

Cek log real-time dari dashboard Render, atau dari lokal:

```powershell
docker compose logs -f app
```

## 12. Checklist sebelum selesai

- [ ] Spreadsheet sudah di-share ke service account sebagai Editor.
- [ ] Google Sheets API aktif.
- [ ] `.env` lokal bisa menjalankan `cmd/setup` tanpa error.
- [ ] `GOOGLE_CREDENTIALS_JSON` di Render valid.
- [ ] `PUBLIC_BASE_URL` = URL Render, HTTPS, tanpa slash.
- [ ] `TELEGRAM_WEBHOOK_SECRET` sama di Render dan Telegram.
- [ ] `TELEGRAM_USER_ID` benar dan user sudah Start bot.
- [ ] `getWebhookInfo` menunjukkan URL Render dan tanpa error.
- [ ] `/healthz` merespons 200.
- [ ] Workflow keep-alive aktif dan secret `HEALTHCHECK_URL` terisi.
- [ ] Kirim pesan uji ke bot dan dapat balasan.

## 13. Keamanan

- Jangan commit `.env` atau file JSON service account.
- Jangan bagikan `TELEGRAM_BOT_TOKEN` dan API key.
- Webhook dilindungi `X-Telegram-Bot-Api-Secret-Token`.
- Bot hanya melayani `TELEGRAM_USER_ID`.
- Jika token/key bocor, revoke di BotFather / Google Cloud, lalu ganti env.
