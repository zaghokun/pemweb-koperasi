# Koperasi Simpan Pinjam

Fondasi **Tahap 1 saja** untuk Koperasi Karyawan Sejahtera. Module Go bernama `koperasi`; dependency utama adalah `github.com/go-sql-driver/mysql` dan `golang.org/x/crypto/bcrypt` (versi terkunci di `go.mod`/`go.sum`). Tersedia migration sembilan tabel dan integration test skema. Belum ada aplikasi HTTP yang dapat dijalankan, login, akun awal, atau password default; fitur tahap berikutnya belum diimplementasikan.

Sumber aturan adalah [RANCANGAN_SISTEM_KOPERASI.md](RANCANGAN_SISTEM_KOPERASI.md), khususnya [KD-01–KD-12](RANCANGAN_SISTEM_KOPERASI.md#22-keputusan-desain-untuk-ketentuan-yang-belum-dirinci-topik), arsitektur Bagian 8, skema Bagian 9, dan urutan tahap Bagian 14. Kebijakan tersebut perlu disetujui pengurus/dosen sebelum transaksi riil; README tidak membuat versi aturan lain.

## Prasyarat dan konfigurasi

- Go **1.26.0 atau lebih baru** yang kompatibel dengan versi dependency terkunci (`go.mod`).
- **MySQL 8.4 LTS**, InnoDB, utf8mb4, strict SQL mode. MariaDB bukan pengganti yang dianggap setara.
- Client `mysql` di PATH; `mysqldump` untuk backup. Administrator MySQL untuk setup database dan akun.

Dari root proyek, di PowerShell:

```powershell
go mod download
Copy-Item .env.example .env
$env:APP_ADDR = '127.0.0.1:8080'
$env:COOKIE_SECURE = 'false'
```

`.env` hanya catatan lokal: **tidak dimuat otomatis** oleh Go/test. Atur environment proses secara eksplisit. `COOKIE_SECURE=false` hanya untuk HTTP localhost; gunakan `true` pada HTTPS ketika aplikasi tersedia.

Bentuk environment DSN (ganti penanda dengan akun/password lokal sendiri; ini bukan kredensial yang berlaku):

```text
DATABASE_DSN=<akun_aplikasi>:<password_lokal>@tcp(127.0.0.1:3306)/koperasi?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4
TEST_DATABASE_DSN=<akun_test>:<password_lokal>@tcp(127.0.0.1:3306)/koperasi_test?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4
```

Di PowerShell, gunakan `$env:DATABASE_DSN = '...'` dan `$env:TEST_DATABASE_DSN = '...'` dengan DSN lokal Anda. Hindari menyimpan rahasia dalam riwayat terminal bersama, screenshot, log, atau Git. `loc=UTC` mengatur parsing driver; `time_zone` mengatur sesi MySQL setiap koneksi. Audit memakai UTC `DATETIME(6)`; tanggal bisnis Asia/Jakarta tetap `DATE`/`YYYY-MM-DD`, bukan instant yang digeser zona waktu.

## Setup database dan akun

Administrator membuat **dua database terpisah**:

```sql
CREATE DATABASE koperasi CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE DATABASE koperasi_test CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
```

Provision akun dengan password unik melalui saluran administratif lokal, bukan berkas yang dilacak. Batasi host dan scope database:

| Akun | Hak minimum untuk pekerjaan ini |
| --- | --- |
| Migrasi | `CREATE`, `ALTER`, `INSERT`, `INDEX`, `REFERENCES` pada `koperasi.*`; MySQL juga memerlukan INSERT untuk ALTER TABLE. Tambah hak hanya bila migration baru membutuhkannya. |
| Aplikasi | `SELECT`, `INSERT`, `UPDATE`, `DELETE` pada `koperasi.*`, tanpa DDL atau akses database test. Hak DELETE diperlukan kelak untuk sesi, bukan izin menghapus histori keuangan lewat aplikasi. |
| Test | `CREATE`, `ALTER`, `INDEX`, `REFERENCES`, `DROP`, `SELECT`, `INSERT`, `UPDATE`, `DELETE` hanya pada `koperasi_test.*`. Tidak boleh mengakses database pengembangan/riil. |

Jangan gunakan akun administrator untuk aplikasi atau test. Database test harus sudah ada dan **kosong**; jangan menjalankan migration manual ke database test sebelum integration test.

## Migration pengembangan

Dari root, jalankan di **CMD atau Bash** (password diminta interaktif):

```sh
mysql --host=127.0.0.1 --user=akun_migrasi --password --batch koperasi < migrations/001_schema.sql && mysql --host=127.0.0.1 --user=akun_migrasi --password --batch koperasi < migrations/002_constraints_indexes.sql
```

Jika memakai PowerShell, buka CMD atau bungkus seluruh perintah di atas dengan `cmd /c '...'`; `<` bukan redirection stdin PowerShell. Urutan wajib `001_schema.sql` lalu `002_constraints_indexes.sql`. Mode batch berhenti pada error dan `&&` mencegah migration kedua berjalan setelah kegagalan; **jangan gunakan `--force`**.

`001` membuat `users`, `sessions`, `members`, `savings`, `loans`, `installments`, `payments`, `payment_allocations`, dan `loan_settlements`; `002` menambah generated guard, unique key kondisional, dan indeks. Ini bukan script reset atau migration otomatis saat startup. MySQL DDL melakukan **implicit commit**: ROLLBACK tidak mengembalikan seluruh migration. Jika gagal, hentikan dan periksa bagian yang sudah terpasang; jangan menghapus data pengguna untuk mengulangnya. Backup sebelum perubahan pada database berisi data. Setelah dibagikan, tambahkan migration bernomor berikutnya, jangan mengubah migration yang sudah diterapkan.

Verifikasi singkat lewat client `mysql` pada database `koperasi`: `SHOW TABLE STATUS;` untuk sembilan tabel/engine, lalu `SHOW CREATE TABLE savings;` dan `SHOW CREATE TABLE loans;` untuk constraint/guard. Pemeriksaan teks atau daftar tabel saja tidak membuktikan aturan skema bekerja.

## Verifikasi fondasi

```powershell
go mod verify
go test -v ./migrations
```

Integration test memerlukan `TEST_DATABASE_DSN` ke MySQL 8.4 sungguhan. Nama database harus ASCII aman dan berakhiran `_test`; gunakan `koperasi_test`. Database khusus tersebut harus **sudah ada, kosong, tidak dipakai proses lain**, dan akun test memiliki hak DDL di atas. Test menolak database tidak kosong, mengambil advisory lock, menerapkan migration, menguji invariant, lalu membersihkan hanya tabel miliknya. Jangan arahkan ke data yang ingin disimpan. Lock test bukan perlindungan terhadap client lain yang tidak mengikuti lock tersebut.

Tanpa environment test, hasil **SKIP bukan bukti lulus**. Fondasi ini telah diverifikasi dengan Go 1.26.6 dan MySQL 8.4.3 terisolasi: `go mod verify` lulus; kedua migration berhasil dengan akun migration berhak terbatas; TestSchema lulus dengan akun test terpisah. Penolakan database test tidak kosong dan DSN database aplikasi juga diperiksa tanpa merusak data. Belum ada test domain/server karena package tersebut belum dibuat.

## Backup dan pemulihan

Cadangkan setiap hari dan sebelum perubahan skema menggunakan `mysqldump --single-transaction` dengan akun backup yang sesuai; jangan ubah skema selama dump. Simpan backup berakses terbatas pada media/perangkat berbeda, bukan Git. Uji restore menggunakan client `mysql` ke database pemulihan terpisah yang disiapkan administrator; gunakan stdin CMD/Bash seperti migration. Periksa tabel, constraint, dan rekonsiliasi sebelum pemakaian. Jangan menimpa database riil atau menyalin direktori data MySQL aktif sebagai backup.

## Berbagi dan bekerja bersama

Lacak `go.mod`, **`go.sum`**, `.env.example`, `.gitignore`, README, **root `AGENTS.md`**, rancangan, serta `migrations/` termasuk test. Jangan bagikan `.env`, DSN/rahasia, dump, atau konfigurasi agent lokal. Gunakan remote/identitas Git yang benar-benar diberikan tim; proyek ini tidak menetapkan remote, akun, commit, atau push otomatis.

Root [AGENTS.md](AGENTS.md) adalah pedoman portabel lintas coding tool. Konfigurasi OMP lokal ada pada `.omp/agents/{koperasi-domain,koperasi-database,koperasi-web,koperasi-review}.md`; seluruh `/.omp/` diabaikan Git sehingga **tidak ikut clone rekan**. Definisi tersebut memakai model aktif yang diwarisi, tanpa pengaturan model/API tersendiri. Gunakan `/agents` di OMP untuk melihat definisinya. Subagent belum tentu otomatis mewarisi root AGENTS; prompt/definisi lokal harus secara eksplisit meminta membacanya beserta bagian rancangan terkait.

Bagi pekerjaan menjadi vertical slice kecil sesuai tahap, tentukan pemilik berkas sebelum mengedit, hormati perubahan rekan, lalu jalankan verifikasi sempit setelah integrasi. Jangan menganggap permintaan satu tahap sebagai izin membangun semua tahap berikutnya.
