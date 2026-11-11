# Pedoman kontributor dan coding agent

## Sumber kebenaran dan batas tugas

Baca [README.md](README.md) dan bagian relevan [RANCANGAN_SISTEM_KOPERASI.md](RANCANGAN_SISTEM_KOPERASI.md) sebelum bekerja. Rancangan memuat aturan bisnis, KD-01–KD-12, kontrak package, dan urutan tahap. Jangan menggandakan kebijakan dengan versi berbeda; laporkan konflik sebelum mengubah perilaku.

**Tahap 1 dan bagian fondasi/jadwal Tahap 2 sudah tersedia:** model bersama, operasi rupiah aman, tanggal bisnis, serta jadwal/periode berjalan telah diintegrasikan dan diuji. Denda, alokasi pembayaran, dan pelunasan dipercepat adalah tugas anggota lain; jangan mengimplementasikannya tanpa penugasan. Belum ada config runtime, pool aplikasi, server HTTP, login, akun pengurus awal, atau UI. Permintaan satu tugas bukan izin mengimplementasikan seluruh rancangan. Buat berkas saat benar-benar dibutuhkan, bukan stub atau abstraksi spekulatif.

Sebelum mengedit, petakan caller, test, fixture, konfigurasi, dan dokumentasi terdampak. Pakai pola yang sudah ada, standard library, dan perubahan terkecil yang menyelesaikan kebutuhan. Hormati perubahan pengguna/kontributor lain; jangan menimpa pekerjaan mereka. Jangan hapus data riil, menjalankan reset destruktif, membuat commit, atau push tanpa permintaan eksplisit. Jangan memasukkan rahasia, dump, `.env`, atau konfigurasi lokal `.omp/` ke Git.

## Module dan tanggung jawab berkas

Module/import internal memakai **`koperasi`**, misalnya `koperasi/internal/domain`, bukan nama folder workstation. Gunakan Go >=1.26.0 yang kompatibel dengan versi terkunci, `database/sql`, `github.com/go-sql-driver/mysql`, dan bcrypt dari `golang.org/x/crypto/bcrypt`; utamakan standard library tanpa ORM/framework tambahan yang tidak diminta. Lacak `go.sum` bersama `go.mod`.

| Lokasi | Tanggung jawab dan status |
| --- | --- |
| `migrations/` | Sudah termasuk Tahap 1: DDL berversi, constraint/indeks, test integrasi skema. Jangan seed transaksi atau password default. |
| `internal/domain/` | Model bersama, uang, tanggal bisnis, serta jadwal/periode berjalan; fungsi murni tanpa SQL/HTTP atau `time.Now()` tersembunyi. Denda/alokasi/pelunasan belum dikerjakan. |
| `internal/config/`, `internal/database/` | **Belum diimplementasikan**: validasi environment; pool/ping/close MySQL. Tidak menjalankan migration saat startup. |
| `internal/repository/` | **Belum diimplementasikan**: SQL berparameter `?`, mapping, agregasi dan row lock melalui DBTX kecil; bukan pemilik commit atau perhitungan bisnis. |
| `internal/service/` | **Belum diimplementasikan**: validasi proses, transaksi, otorisasi, urutan lock, pemanggilan domain/repository. |
| `internal/middleware/`, `internal/handler/`, `internal/router/` | **Belum diimplementasikan**: sesi/CSRF; parse form dan render; penyambungan route. Bukan lokasi perhitungan uang/SQL. |
| `cmd/server/` | **Belum diimplementasikan**: wiring dependency dan lifecycle server. |
| `web/templates/`, `web/static/` | **Belum diimplementasikan**: `html/template`, Bootstrap/aset lokal; bukan satu-satunya validasi. |

Struktur terperinci berada pada Bagian 8 rancangan; daftar tersebut bukan perintah membuat semua direktori sekarang. Letakkan unit test dekat implementasi, bukan package test generik baru.

Kontrak domain dan pembagian berkas Tahap 2 berada di README. Jangan mendefinisikan ulang model atau operasi uang di berkas anggota lain. Tanggal bisnis pada domain memakai komponen kalender dalam time.Time UTC tengah malam sebagai pembawa tanggal, bukan instant UTC: normalisasi mempertahankan tahun/bulan/hari tanpa `.In(...)`. Konversi instant riil ke Asia/Jakarta dilakukan pemanggil sebelum mengambil tanggal; tanggal nullable memakai *time.Time dan ID nullable *int64.

## Invariant Go/MySQL dan bisnis

- Gunakan **MySQL 8.4 LTS**, InnoDB, utf8mb4 dan strict SQL mode; jangan mengganti dengan MariaDB, SQLite, atau mock SQL sebagai bukti kompatibilitas MySQL.
- Uang rupiah utuh menggunakan `int64` Go/`BIGINT` signed MySQL; ID/FK juga signed BIGINT. Jangan memakai float. Periksa overflow penjumlahan/perkalian/konversi. Pembulatan half-up dan sisa pokok angsuran terakhir mengikuti KD-01.
- Jasa flat berasal dari **pokok awal**, bukan saldo menurun. Denda, pembayaran sebagian, prioritas global **denda → jasa → pokok** dengan angsuran tertua per komponen, periode berjalan, pelunasan dan urutan tanggal mengikuti KD-02–KD-12/Bagian 5–7. Jangan menyederhanakan kebijakan diam-diam.
- Audit/expiry memakai UTC `DATETIME(6)`. Tanggal bisnis Asia/Jakarta memakai `DATE`/`YYYY-MM-DD`; jangan menggeser komponen tanggal karena konversi zona waktu. DSN memakai `parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4` di setiap koneksi pool.
- Mutasi keuangan berada dalam satu transaksi `READ COMMITTED`: lock anggota dahulu, pinjaman berikutnya, lalu angsuran menurut nomor; baca ulang kewajiban dan tulis memakai **tx yang sama**. Service memegang begin/commit/rollback; tangani kegagalan commit. Detail idempotensi, deadlock dan lock timeout mengikuti Bagian 10.
- Database sumber kebenaran; constraint dan lock saling melengkapi. SQL memakai placeholder dan daftar kolom eksplisit; jangan memasukkan generated column sebagai input. Validasi trust boundary di server, termasuk nominal, tanggal, akses, dan overflow.
- Histori keuangan yang sudah commit tidak diedit/dihapus lewat endpoint. Jangan menghapus histori untuk membuat test/migration berhasil. Implementasi keamanan HTTP kelak mengikuti Bagian 10, bukan boilerplate sekarang.

## Migration dan pemeriksaan

Urutan saat ini `001_schema.sql` lalu `002_constraints_indexes.sql`. Setelah migration dibagikan/diterapkan, **jangan ubah isinya**; buat migration baru bernomor berikutnya. DDL MySQL implicit commit, bukan transaksi rollback utuh. Backup sebelum perubahan skema berisi data; saat gagal hentikan dan periksa keadaan, jangan reset data pengguna. Pisahkan akun migrasi, aplikasi tanpa DDL, serta akun test khusus sesuai README.

Perintah yang tersedia saat ini:

```text
go mod download
go mod verify
go test ./internal/domain
go test -v ./migrations
```

Test skema memerlukan `TEST_DATABASE_DSN` pada MySQL 8.4 dengan database khusus **sudah ada dan kosong**, nama ASCII aman berakhiran `_test`, serta hak DDL/DML terbatas di database itu. Test mengambil advisory lock, menolak database tidak kosong, dan hanya membersihkan tabel miliknya. Ikuti README; jangan pakai database pengembangan/riil. Hasil SKIP tanpa environment **bukan** bukti skema lulus. Jangan mengklaim pemeriksaan berhasil tanpa output nyata.

`go test ./internal/domain` memeriksa bagian domain yang sudah diimplementasikan dan tidak membutuhkan MySQL atau `.env`. Lulusnya test ini bukan berarti seluruh Tahap 2 selesai; denda, alokasi, dan pelunasan harus ditambahkan beserta test oleh pemiliknya. Untuk perubahan nontrivial sertakan test kecil yang relevan. Pada pekerjaan beragen, worker tidak menjalankan build/test/lint/formatter saat pengeditan; pemilik integrasi menjalankan pemeriksaan setelah perubahan digabung. Laporkan pemeriksaan yang benar-benar dijalankan, tanpa klaim kesiapan transaksi riil.

## Kolaborasi dan OMP

Kerjakan vertical slice sesuai urutan tahap: aturan dan penyimpanan sampai antarmuka yang diperlukan, bukan semua handler lebih dahulu. Tetapkan pemilik berkas/interface secara eksplisit, hindari edit bersama pada area yang sama, dan verifikasi sempit setelah integrasi. Perbarui caller/test/docs yang terkena perubahan; jangan menambah fitur, opsi, telemetry, atau dependency untuk kebutuhan yang belum ada.

Root AGENTS ini dibagikan dan portabel lintas coding tool. Definisi OMP lokal `.omp/agents/{koperasi-domain,koperasi-database,koperasi-web,koperasi-review}.md` tidak ikut clone karena `/.omp/` diabaikan. Definisi memakai model aktif yang diwarisi, tanpa konfigurasi model/API tersendiri; lihat melalui `/agents`. Subagent OMP **belum tentu mewarisi AGENTS ini otomatis**: setiap prompt/definisi harus secara eksplisit meminta membaca root AGENTS dan bagian rancangan yang relevan. Jangan menganggap konfigurasi lokal rekan tersedia atau mengubah definisi lokal menjadi kebijakan proyek tersembunyi.
