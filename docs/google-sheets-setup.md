# Setup Google Sheets

Database bot ini adalah satu spreadsheet Google. Autentikasi memakai **service
account** (tanpa OAuth flow dan tanpa refresh token).

## 1. Buat project & aktifkan API

1. Buka [Google Cloud Console](https://console.cloud.google.com/).
2. Buat project baru (misal `one-more-percent`).
3. Menu **APIs & Services → Library**, cari **Google Sheets API**, klik
   **Enable**.

## 2. Buat service account

1. **APIs & Services → Credentials → Create Credentials → Service account**.
2. Beri nama, lanjutkan, lalu **Done**.
3. Buka service account → tab **Keys** → **Add key → Create new key → JSON**.
4. File JSON terunduh. File ini berisi `client_email`, `private_key`, dan
   `token_uri`. **Jangan commit file ini.**

## 3. Siapkan spreadsheet

1. Buat spreadsheet di Google Sheets.
2. Klik **Share**, tempel `client_email` dari file JSON, pilih **Editor**.
3. Ambil ID spreadsheet dari URL:
   `https://docs.google.com/spreadsheets/d/<ID>/edit`.

## 4. Isi environment

```env
SHEETS_SPREADSHEET_ID=<ID spreadsheet>
GOOGLE_CREDENTIALS_FILE=./google-credentials.json
GOOGLE_SCOPE=https://www.googleapis.com/auth/spreadsheets
# atau untuk hosting tanpa file:
# GOOGLE_CREDENTIALS_JSON={"type":"service_account",...}
```

`GOOGLE_CREDENTIALS_JSON` harus berisi JSON mentah satu baris (boleh memakai
newline yang di-escape `\n` pada private key).

## 5. Buat worksheet & seed

```bash
go run ./cmd/setup
```

Perintah ini membuat worksheet yang belum ada beserta header-nya:

- `schedules`: `id, type, date, day_of_week, start_time, end_time, activity, active`
- `progress_logs`: `id, date, schedule_id, activity, status, note, created_at`
- `reminder_state`: `date, schedule_id, type, sent_at`
- `progress_archive`: kolom sama dengan `progress_logs`

Jika `schedules` masih kosong, data contoh 56 jadwal ikut diisi. Lewati seed
dengan `go run ./cmd/setup -seed=false`.

## 6. Tipe jadwal

| type | date | day_of_week | Arti |
| --- | --- | --- | --- |
| `weekly` | kosong | `Monday`..`Sunday` | berulang tiap minggu |
| `daily` | kosong | kosong | berulang tiap hari |
| `once` | `YYYY-MM-DD` | kosong | hanya sekali pada tanggal itu |

- `start_time`/`end_time` format `HH:MM` (24 jam).
- `active` berisi `TRUE`/`FALSE`.
- Jadwal `once` yang sudah lewat otomatis dinonaktifkan oleh maintenance harian.
- `id` harus unik dan berupa angka; biarkan sistem yang membuat saat lewat chat.

## 7. Kenapa spreadsheet tidak penuh

Jadwal berulang **tidak** membuat baris baru setiap hari. Yang bertambah hanya
`progress_logs` dan `reminder_state` (satu baris per kejadian). Maintenance
harian:

- memindahkan `progress_logs` lebih tua dari `PROGRESS_RETENTION_DAYS` (default
  90 hari) ke `progress_archive`, lalu menghapusnya dari sheet utama;
- menghapus `reminder_state` lebih tua dari `REMINDER_RETENTION_DAYS`
  (default 7 hari);
- menonaktifkan jadwal `once` yang sudah lewat.

Dengan begitu sheet utama tetap kecil. Arsip tumbuh sangat lambat (satu baris
per aktivitas yang benar-benar dikerjakan) dan masih jauh di bawah limit
10 juta sel Google Sheets.

## 8. Migrasi data lama (opsional)

Kalau sebelumnya pernah memakai PostgreSQL, ekspor dulu:

```sql
\copy (SELECT id, 'weekly', '', day_of_week, to_char(start_time,'HH24:MI'),
       to_char(end_time,'HH24:MI'), activity, is_active
       FROM schedules ORDER BY id) TO 'schedules.csv' CSV
```

Lalu tempel isinya ke worksheet `schedules` mulai baris 2 dengan urutan kolom
`id, type, date, day_of_week, start_time, end_time, activity, active`.
Untuk `schedule_progresses`, petakan ke kolom `progress_logs`
(`id, date, schedule_id, activity, status, note, created_at`). Bot tidak
melakukan migrasi otomatis.

## 9. Troubleshooting

- `403 PERMISSION_DENIED`: spreadsheet belum di-share ke `client_email`.
- `Unable to parse range`: nama sheet di env tidak cocok dengan judul tab.
- `invalid_grant`: jam server salah atau private key rusak/ter-escape.
- `429 RESOURCE_EXHAUSTED`: kuota Sheets API. Bot otomatis retry + backoff.
  Kurangi frekuensi dengan menaikkan `SHEETS_CACHE_TTL_SECONDS`.
