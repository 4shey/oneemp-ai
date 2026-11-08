# AI Providers

Bot memakai interface `ai.Client`, jadi provider bisa diganti tanpa mengubah
kode. Pilih lewat `AI_PROVIDER`.

## Groq (alternatif)

```env
AI_PROVIDER=groq
GROQ_API_KEY=gsk_...
GROQ_BASE_URL=https://api.groq.com/openai/v1
GROQ_MODEL=openai/gpt-oss-120b
```

Bot memanggil `POST {GROQ_BASE_URL}/chat/completions` dengan header
`Authorization: Bearer`. Nama model bisa diganti dengan model Groq lain yang
tersedia di dashboard Groq.

## Google AI Studio (Gemini, default)

```env
AI_PROVIDER=gemini
GEMINI_API_KEY=...
GEMINI_BASE_URL=https://generativelanguage.googleapis.com/v1beta
GEMINI_MODEL=gemini-3.1-flash-lite
```

Nama model Gemini berubah dari waktu ke waktu. Cek daftar model yang tersedia
untuk API key Anda lewat endpoint `GET {GEMINI_BASE_URL}/models`, lalu isi
`GEMINI_MODEL` dengan nama yang mendukung `generateContent`.

Bot memanggil:

```
POST {GEMINI_BASE_URL}/models/{GEMINI_MODEL}:generateContent
x-goog-api-key: <GEMINI_API_KEY>
```

Pemetaan otomatis:

- `System` → `systemInstruction`
- role `assistant` → `model`
- `JSON=true` → `generationConfig.responseMimeType=application/json`

## Perilaku saat gagal

- HTTP 429 / 5xx di-retry dengan exponential backoff
  (`SHEETS_MAX_RETRIES`, `SHEETS_RETRY_BASE_MS`).
- Jika tetap gagal:
  - chat membalas pesan ramah "kuota AI penuh / model tidak bisa dihubungi";
  - reminder memakai pesan fallback deterministik;
  - recap dicoba lagi setelah cooldown 5 menit.
- JSON hasil AI divalidasi sebelum ada penulisan ke spreadsheet. JSON rusak
  tidak pernah menghasilkan perubahan data.

## Menambah provider baru

Lihat `AGENTS.md` bagian 7. Singkatnya: buat `internal/ai/<nama>`, implement
`ai.Client`, daftarkan di `internal/ai/factory/factory.go`, tambah env.
