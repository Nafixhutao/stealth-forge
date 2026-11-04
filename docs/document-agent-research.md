# Document Agent — Fase 0: Riset & Keputusan Arsitektur

Status: **DRAFT — menunggu persetujuan sebelum Fase 1.**
Tanggal: 2026-11-02

> **Update 2026-11-02 (keputusan owner):** runtime agent diimplementasikan dalam
> **Rust** (`agent-rs/`), bukan Go. Worker Go tetap memegang queue, lease, log,
> cancel, dan persistensi hasil; ia memanggil binary Rust sebagai child process.
> LLM memakai **RinsDev Cloud** (`deepseek-v4.1-flash-1017`). Arsitektur lain
> (sandbox Docker dikeraskan, DOCX via python-docx, PDF via LibreOffice, preview
> pdf.js, tabel `agent_artifacts`) tetap berlaku.

Dokumen ini adalah keluaran Fase 0. Isinya: temuan riset terhadap `openai/codex`
dan kode Stealth, tabel perbandingan library, keputusan arsitektur beserta
alasannya, diagram alur data, dan rencana fase berikutnya dengan estimasi.

---

## 1. Ringkasan temuan

### 1.1 Celah yang akan diisi

Stealth sudah mengimplementasikan seluruh **siklus hidup Agent** — konfigurasi,
katalog, durable run, log, dan cancel — tetapi **eksekusi masih queue-only** karena
tidak ada provider adapter yang terdaftar. Titik integrasi yang kosong sudah
dibentuk dengan jelas dan siap diisi:

- `internal/agentrunner/worker.go` mendefinisikan kontrak `Adapter`:

  ```go
  type Adapter interface {
      Execute(context.Context, Job) (repository.AgentRunResult, error)
  }
  ```

- `cmd/worker/main.go:326` membuat `agentrunner.NewRegistry()` **kosong**, sehingga
  worker tidak pernah meng-claim run. TODO ini adalah celah yang jadi rumah natural
  untuk Document Agent.

Artinya kita **tidak perlu menciptakan antrian baru** — cukup mengimplementasikan
`Adapter` dan mendaftarkannya ke `Registry`. Ini keputusan besar yang menyederhanakan
Fase 3 secara signifikan.

### 1.2 Temuan kunci dari `openai/codex` (codex-rs)

- **License**: Apache-2.0 (`LICENSE`, `Copyright 2025 OpenAI`) + `NOTICE`
  (atribusi Ratatui/MIT). Syarat redistribusi: (a) sertakan COPYING; (b) tandai file
  yang diubah; (c) pertahankan header copyright/patent/trademark; (d) sertakan isi
  `NOTICE`.
- **PENTING**: bubblewrap yang di-vendor di `codex-rs/vendor/bubblewrap/` adalah
  **LGPL-2.1** (`COPYING`). Kalau kita membundel biner bwrap dari Codex, artikel
  redistribusi LGPL berlaku (harus bisa relink). Kami **tidak** menyalin biner ini;
  kami hanya meminjam desain.
- **Agent loop** (`core/src/session/turn.rs:run_turn`): pola `sample model → eksekusi
  tool call → append hasil output tool ke history → sample lagi` sampai model berhenti
  memanggil tool. Tidak ada "channel file" khusus — output tool ditulis balik ke
  history percakapan.
- **Tool sebagai spesifikasi + runtime**: `ToolExecutor` trait + `ToolSpec` (Function /
  Freeform). `shell` = `exec_command` (argv `bash -lc "<cmd>"`); `apply_patch` = tool
  *freeform grammar* (lark) untuk edit/tulis file.
- **Sandbox = transform argv**: hot path re-exec `codex-linux-sandbox` yang
  menggerakkan bubblewrap (`--ro-bind /`, `--bind <workdir>`, read-only by default) +
  seccomp (blokir syscall socket) + `--unshare-user/pid/ipc/net` + `--cap-drop ALL` +
  `--no-new-privs`.
- **Batas resource Codex hanya** waktu + ukuran output + jumlah proses
  (`--pids-limit`-like) — **TIDAK ada** limit CPU/memori/rlimit file size. Ini justru
  kelemahan yang akan kami perbaiki dengan sandbox Docker (lihat §3.2).

### 1.3 Temuan kunci dari kode Stealth

- **Tidak ada objek "artifact" untuk Agent.** Istilah "artifact" di repo ini hanya
  milik Apps/Functions/Sites (arsip build, OCI image, objek storage). Fase 3 harus
  **menambah** tabel/domain/endpoint artifact baru untuk Agent.
- **Storage adalah primitif yang bersih** (`internal/storage`: `BlobStore` lokal +
  S3), path buram `projectID/bucketID/fileID`, unduh via `GET .../files/{fileID}/download`
  + `http.ServeContent` (tanpa presigned URL).
- **Event realtime** = outbox PostgreSQL (`webhook_events`) → Redis pub/sub → SSE.
  Penamaan mengikuti konvensi verb: `agent.create`, `agent.run.*`, `storage_file.*`.
  Kita perlu menambah prefix fanout baru (mis. `document.artifact.`).
- **Sandbox Docker sudah ada** di `internal/functionrunner/docker.go`: container
  ephemeral `--rm` dengan `--network=none --read-only --cap-drop=ALL
  --security-opt no-new-privileges:true --pids-limit 128 --memory 256m --cpus 1
  --user 65532:65532` + tmpfs. Pola ini hampir persis dengan kebutuhan Fase 2 dan
  hanya perlu ekstensi (batas disk/file-size + user writable workdir terpisah).
- **Batas keamanan jelas**: hanya worker yang punya akses `/var/run/docker.sock`;
  API tidak (`compose.production.yaml:261-297` vs `357-408`).

---

## 2. Lisensi skill dokumen anthropics/skills (keputusan penting)

Poin 4 Fase 0 meminta pengecekan lisensi skill `docx`/`pdf` dari `anthropics/skills`
sebelum dipakai di produk.

**Temuan**: skill dokumen (`docx`, `pdf`, `pptx`, `xlsx`) dari `anthropics/skills`
adalah **source-available, BUKAN open source** (dikonfirmasi dari README repo dan
`LICENSE.txt` yang terpasang lokal):

> "Many skills in this repo are open source (Apache 2.0). We've also included the
> document creation & editing skills ... in the skills/docx, pdf, pptx, and xlsx
> subfolders. These are source-available, not open source."

`LICENSE.txt` skill `docx` menyatakan larangan eksplisit:

- membuat derivative works,
- mendistribusikan / menyublisensikan materi,
- mengekstraksi / menyimpan salinan di luar Services,
- reverse engineer.

**Keputusan**: kami **TIDAK boleh** menyalin kode/script/aset dari skill `docx`/`pdf`
Anthropic ke produk Stealth (lisensi Apache-2.0). Yang boleh dipinjam hanyalah
**pendekatan/ide** (mis. "tulis script python-docx, konversi DOCX→PDF via
`soffice --headless`"), bukan implementasinya. Kami menulis implementasi sendiri
dari nol. Tidak ada `NOTICE`/atribusi Anthropic yang diperlukan karena tidak ada kode
yang disalin.

---

## 3. Perbandingan library & keputusan

### 3.1 Klien LLM & tool calling (bahasa agent)

| Opsi | Kelebihan | Kekurangan | Kematangan | Lisensi | Risiko |
| --- | --- | --- | --- | --- | --- |
| **Go di dalam worker** (loop + OpenAI-compatible SDK, direkomendasikan) | Tanpa proses/binary baru; reuse queue/lease/log/cancel/Storage langsung; deploy = satu Go binary; konsisten dengan `agentrunner.Adapter` | Ekosistem tool-calling Go lebih tipis dari Python/Node | SDK Go OpenAI resmi (openai-go) beta-stabil | Apache-2.0 (openai-go) | Membangun loop sendiri (~200-400 LOC) tapi sederhana |
| **Rust proses terpisah** (gaya Codex) | Isolasi sandbox terkuat; codex-rs jadi referensi | Toolchain build Rust + IPC + packaging + satu proses lagi di VM 4-8GB; effort jauh lebih besar | Tinggi (di Codex) | Apache-2.0 + LGPL-2.1 (bwrap) | Overkill untuk 1 VPS; biaya ops & kepatuhan LGPL |
| **Python agent** (OpenAI SDK / LiteLLM) | Ekosistem tool-calling paling lengkap; LiteLLM = multi-provider | Proses/venv tambahan; sandbox Python beda dari control plane Go | Tinggi | MIT (LiteLLM) | Lifecycle & logging harus dijembatani ke Go |
| **Node agent** (OpenAI SDK / Vercel AI SDK) | AI SDK sangat matang untuk tool calling & streaming | Proses Node tambahan; duplikasi dengan console | Tinggi | MIT / Apache-2.0 | Sama seperti Python |

**Keputusan: Go di dalam worker**, mengimplementasikan `agentrunner.Adapter` yang
berisi loop tool-calling sederhana. Klien LLM memakai SDK Go OpenAI-compatible
(`openai-go`) yang juga menopang endpoint chat-completion berbagai provider
self-host (katalog `provider`/`model` sudah ada). Alasannya:
(1) titik integrasi `Adapter` sudah disiapkan; (2) semua primitif (queue, log, cancel,
storage, event) ada di Go; (3) target 1 VPS 2-4 vCPU/4-8GB — proses Rust/Node/Python
tambahan tidak sebanding dengan kompleksitas yang ditambahkan. Opsi Rust dicatat
sebagai rencana cadangan bila batas isolasi Docker ternyata kurang.

### 3.2 Sandbox eksekusi

| Opsi | Kelebihan | Kekurangan | Kematangan | Lisensi | Risiko |
| --- | --- | --- | --- | --- | --- |
| **Docker dikeraskan (reuse `functionrunner`)** — direkomendasikan | Sudah ada & teruji di repo; limit eksplisit (`--memory/--cpus/--pids-limit/--ulimit` + tmpfs/disk quota); image contain Python+docx+LibreOffice+font; satu teknologi yang sudah dipakai platform | Isolasi kernel bersama (bukan "kernel terpisah"); butuh docker.sock (hanya di worker) | Tinggi | Apache-2.0 (Docker/Moby) | CVE kernel; mitigasi: `--security-opt no-new-privileges`, read-only rootfs, non-root, drop cap |
| **bubblewrap/Landlock/seccomp (gaya Codex)** | Tanpa daemon; user-namespace; read-only fs default | Landlock butuh kernel ≥5.13; konfigurasi rumit; Codex TIDAK batasi CPU/mem/disk (harus kami tambah sendiri via rlimit/cgroup) | Tinggi (di Codex) | bwrap LGPL-2.1, selebihnya Apache/MIT | Lebih banyak kode sendiri; kepatuhan LGPL jika bundling bwrap |
| **gVisor (runsc)** | Isolasi syscall userspace; tamu kernel sendiri | Runtime terpisah; overhead; batas fleksibilitas fitur | Tinggi | Apache-2.0 | Ops berat untuk 1 VPS; image harus kompatibel runsc |

**Keputusan: Docker dikeraskan**, me-reuse pola `functionrunner.DockerExecutor` dan
memperluasnya untuk kebutuhan Document Agent: satu workdir writable (`--mount
type=volume`), rootfs read-only, `--network=none`, user non-root, eksplisit
`--memory/--memory-swap/--cpus/--pids-limit/--ulimit nofile/fsize/cpu`, tmpfs
berukuran untuk `/tmp`, dan disk quota workdir. bubblewrap/Landlock dicatat sebagai
_opsional defense-in-depth_ untuk rilis lanjutan, bukan pengganti — karena Codex
sendiri tidak menutup celah resource limit yang kita butuhkan.

Download DOCX/PDF ke konsol tidak lewat docker.sock melainkan lewat endpoint Storage
yang sudah ada.

### 3.3 Pembuatan DOCX

| Opsi | Kelebihan | Kekurangan | Kematangan | Lisensi | Risiko |
| --- | --- | --- | --- | --- | --- |
| **python-docx** — direkomendasikan | De-facto standar; API stabil; didukung model LLM dengan baik; image sandbox sudah Python | Fitu tabel/daftar butuh atensi (footgun lebar kolom DLL) | Sangat tinggi | MIT | Render tidak 1:1 antar viewer (mitigasi: konversi LibreOffice utk preview) |
| **docx (Node, docx-js)** | API kaya; tanpa binary native | Perlu runtime Node di sandbox; footgun table width | Tinggi | MIT | Sama, ditambah runtime Node ekstra |
| **docx-rs (Rust)** | Typed, cepat | Ekosistem kecil; LLM jarang hafal API-nya | Sedang | MIT | LLM lebih sering salah pakai API |

**Keputusan: `python-docx`** di dalam sandbox (Python + LibreOffice sudah satu image).
DOCX dihasilkan oleh tool tulis-file yang menjalankan script Python di sandbox, lalu
di-"render" jadi PDF via LibreOffice untuk preview.

### 3.4 Pembuatan PDF

| Opsi | Kelebihan | Kekurangan | Kematangan | Lisensi | Risiko |
| --- | --- | --- | --- | --- | --- |
| **LibreOffice headless (DOCX→PDF)** — direkomendasikan | Satu sumber kebenaran (DOCX); render konsisten; tanpa belajar bahasa markah baru; dipakai industri | Lebih lambat (~1-3s/doc); font harus ada di image; hasil tergantung versi LO | Sangat tinggi | MPL-2.0 | Kesetiaan format minor; mitigasi: uji golden file |
| **Typst** | PDF native indah + cepat; sintaks sederhana; deterministik | LLM harus belajar syntax Typst; bukan DOCX (dua sumber) | Tinggi, cepat berkembang | Apache-2.0 | Konten kompleks (tabel/diagram) butuh belajar |
| **HTML → PDF (headless Chromium/WeasyPrint)** | Familiar bagi LLM (HTML/CSS) | Headless browser berat di sandbox; determinisme rendah | Tinggi | Chromium BSD, WeasyPrint BSD | Ukuran image besar; output tak deterministik |

**Keputusan: DOCX sebagai artefak kanonik, PDF sebagai turunan via
`libreoffice --headless --convert-to pdf`.** Alur "buat surat jadi Word" →
`.docx` → konversi ke PDF untuk preview dan unduhan PDF. Ini menyederhanakan Fase 4
("PDF dipreview langsung; DOCX dikonversi ke PDF untuk preview"). Typst dipertimbangkan
hanya bila nanti dibutuhkan dokumen PDF native tanpa DOCX.

### 3.5 Preview di console & konversi DOCX→PDF

| Opsi | Kelebihan | Kekurangan | Kematangan | Lisensi | Risiko |
| --- | --- | --- | --- | --- | --- |
| **pdf.js (Mozilla)** — direkomendasikan | Render PDF akurat lintas browser & HP; kontrol penuh (zoom, antarmuka, mobile); sudah dipakai luas | Perlu build worker/asset; integrasi React manual | Sangat tinggi | Apache-2.0 | Ukuran bundle; mitigasi code-split |
| **iframe/`<embed>` PDF native** | Nol dependensi | Tampilan tak konsisten lintas browser; minim kontrol di mobile | Tinggi | N/A | UX jelek di HP; tidak cocok panel dua kolom |
| **docx-preview.js** (DOCX langsung) | Preview DOCX tanpa konversi | Fidelity < LibreOffice; tak mendukung pdf | Sedang | MIT | Render tabel/gaya bisa menyimpang |

**Keputusan: pdf.js** untuk merender PDF. Preview DOCX = konversi server-side ke PDF
via LibreOffice lalu dirender pdf.js; file `.docx` asli tetap tersedia untuk diunduh.
`<iframe>` dicatat sebagai fallback cepat bila integrasi pdf.js tertunda.

---

## 4. Arsitektur yang diusulkan (keputusan + alasan)

1. **Document Agent = provider adapter Go** di `internal/agentrunner`, di-register ke
   `Registry` di `cmd/worker`. Mengisi celah `Adapter` yang sudah disiapkan; tidak ada
   antrian atau proses baru.
2. **Loop tool-calling** (gaya Codex) dengan dua tool: `shell` dan `write_file` (versi
   minimal dari `apply_patch`/`exec_command`). Model menghasilkan panggilan tool,
   output tool ditulis balik ke history, lalu model dipanggil ulang sampai selesai.
3. **Batas per run** eksplisit: maksimal N langkah, maksimal token per langkah, timeout
   total (nilai di config; default TBD di Fase 1).
4. **Sandbox = container Docker dikeraskan** per run, gambar berisi Python, python-docx,
   LibreOffice, dan font; dihancurkan setelah run. Batas CPU/memori/proses/waktu/ukuran
   file eksplisit (melebihi Codex yang tidak membatasi CPU/mem/disk).
5. **Output**: DOCX kanonik; PDF turunan via LibreOffice; keduanya diunggah ke Storage
   (`BlobStore`) dan dicatat sebagai baris artifact baru (tabel `agent_artifacts`),
   bukan `output_text`.
6. **Realtime**: tambah prefix fanout `document.artifact.` + event `artifact.created`
   mengikuti pola `enqueueWebhookEventTx` yang sudah ada.
7. **Satu provider LLM dulu**, OpenAI-compatible, terisolasi sebagai config/secret
   proses-lokal worker (tidak pernah disimpan/di-log; tidak di `NEXT_PUBLIC_*`).
8. **Target 1 VPS (2-4 vCPU, 4-8GB)**: semua limit resource literal, ukuran output
   dibatasi, dan tidak ada layanan tambahan wajib (Docker sudah ada).

### Kepatuhan lisensi / keamanan yang ditegakkan

- **Codex**: hanya desain yang dipinjam; tidak ada kode/biner yang disalin, sehingga
  tidak ada kewajiban NOTICE/atribusi. Bila kelak menyalin potongan kode, wajib
  pertahankan header Apache-2.0 + tandai file yang diubah; **hindari** biner bwrap
  (LGPL-2.1).
- **Anthropic skills**: dilarang dipakai secara langsung (proprietary). Implementasi
  ditulis mandiri; hanya pendekatan yang diikuti.
- **Dependensi baru** (python-docx, pdf.js, openai-go, dll) dicatat lisensinya di
  §5 dan di ulas ulang saat masuk (seperti dependensi lain, per `AGENTS.md`).
- **Prompt injection**: isi dokumen dan output tool diperlakukan sebagai data tak
  tepercaya; tool tidak pernah diberi kredensial/secret; batas sandbox mutlak.

---

## 5. Diagram alur data

```mermaid
flowchart TD
    U[Pengguna] -->|chat| C[Console Next.js]
    C -->|POST /v1/agents/{id}/runs| API[Go API]
    API -->|persist prompt| DB[(PostgreSQL agent_runs status=queued)]
    API -->|event agent.run.accepted| OUT[(webhook_events outbox)]
    OUT --> RP[realtimepublisher] --> Redis[(Redis pub/sub)] --> SSE[SSE ke Console]

    RQ[agent_runs queue] -->|ClaimNextAgentRunForProviders| W[Worker agentrunner]
    W -->|Register Adapter| DA[DocumentAgentAdapter Go]
    DA -->|loop tool calling| LLM[OpenAI-compatible LLM]
    DA -->|tool: shell / write_file| SX[Sandbox Docker dikeraskan]
    SX -->|python-docx| DOCX[document.docx]
    SX -->|soffice --headless| PDF[document.pdf]
    DOCX --> ST[(Storage BlobStore lokal / S3)]
    PDF --> ST
    DA -->|AppendAgentRunLog| LOG[(agent_run_logs)]
    DA -->|create artifact rows| ART[(agent_artifacts)]
    ART --> ST
    DA -->|transition run completed| DB
    DA -->|event document.artifact.created| OUT
    ART -->|GET files/{id}/download| API2[Go API ServeContent] --> C
    C -->|pdf.js preview + tombol unduh| U
```

Alur revisi (Fase 4) mengikuti jalur yang sama: prompt revisi baru → run baru →
artifact **versi baru** (baris `agent_artifacts` dengan `version` naik), versi lama
tetap tersimpan dan ditampilkan lewat pemilih versi.

---

## 6. Skema artifact yang diusulkan (pratinjau Fase 3)

```sql
CREATE TABLE agent_artifacts (
  id UUID PRIMARY KEY,
  run_id UUID NOT NULL,            -- FK ke agent_runs
  project_id UUID NOT NULL,
  kind TEXT NOT NULL,              -- 'document'
  title TEXT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  mime_type TEXT NOT NULL,         -- application/vnd...docx | application/pdf
  storage_path TEXT NOT NULL,      -- path buram BlobStore
  size_bytes BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (run_id, kind, version)
);
```

(Rincian final — FK, CHECK, indeks, DTO domain, endpoint — dikerjakan di Fase 3
dalam satu migration bernomor baru.)

---

## 7. Rencana fase berikutnya & estimasi

| Fase | Lingkup | Estimasi |
| --- | --- | --- |
| **Fase 1** — agent inti (terminal) | Loop tool-calling Go + tools `shell` & `write_file`; 1 provider; batas langkah/token; lolos "buat surat lamaran → .docx terbuka di Word & LibreOffice" | 1–2 hari |
| **Fase 2** — sandbox | Sandbox Docker dikeraskan (no network, workdir writable, rootfs read-only, non-root, limit CPU/mem/proc/time/file-size); 1 sandbox/run; image Python+docx+LO+font; tes otomatis kontra (baca luar workdir, akses jaringan, fork bomb, isi disk) gagal & pembuatan dokumen sukses | 2–3 hari |
| **Fase 3** — integrasi Stealth | `DocumentAgentAdapter` ambil run dr queue, tulis log, hormati cancel; upload ke Storage; tabel/domain/endpoint artifact + daftar/unduh; event `artifact.created`; update `openapi.yaml` (generate ulang client); "1 run → artifact terunduh via API" | 2–3 hari |
| **Fase 4** — tampilan console | Layout dua kolom (chat kiri, artifact kanan, tab Preview/Source, unduh PDF+DOCX, pemilih versi; HP = full page); preview pdf.js; revisi lewat chat → versi baru; alur end-to-end di desktop & HP | 2–3 hari |

**Total estimasi kasar**: 7–11 hari kerja untuk MVP end-to-end, dengan Fase 1–3
sebagai milestone "bisa diverifikasi lewat API" dan Fase 4 sebagai milestone
"bisa dipakai di browser".

---

## 8. Kepatuhan lisensi dependensi baru (daftar awal)

| Dependensi | Kegunaan | Lisensi | Catatan |
| --- | --- | --- | --- |
| openai-go (SDK Go) | klien LLM/tool-calling | Apache-2.0 | belum ditambahkan |
| python-docx | buat DOCX di sandbox | MIT | di image sandbox |
| LibreOffice | DOCX→PDF & render | MPL-2.0 | di image sandbox |
| pdf.js | preview PDF di console | Apache-2.0 | belum ditambahkan |
| Docker/Moby + minio-go | sandbox + storage (sudah ada) | Apache-2.0 | sudah di repo |

Rincian dan pencatatan resmi dependensi dilakukan saat dependensi benar-benar
ditambahkan di fase implementasi, mengikuti proses ulasan dependensi repo.

---

## 9. Pertanyaan untuk persetujuan (Fase 0 → Fase 1)

1. Setuju agent ditulis **di Go dalam worker** (bukan Rust/Python/Node terpisah)?
2. Setuju sandbox memakai **Docker dikeraskan** (reuse `functionrunner`), bukan
   bubblewrap/Landlock/gVisor?
3. Setuju **DOCX kanonik + PDF via LibreOffice** (bukan Typst/HTML→PDF)?
4. Setuju preview memakai **pdf.js** (bukan iframe/docx-preview.js)?
5. Setuju penambahan tabel **`agent_artifacts`** baru (bukan memakai ulang kolom
   `output_text` dari `agent_runs`)?