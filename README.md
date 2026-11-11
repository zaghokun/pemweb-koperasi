# Koperasi Simpan Pinjam

Proyek Koperasi Karyawan Sejahtera dengan **Tahap 1 tersedia dan Tahap 2 dikerjakan per bagian**. Module Go bernama `koperasi`; dependency utama adalah `github.com/go-sql-driver/mysql` dan `golang.org/x/crypto/bcrypt` (versi terkunci di `go.mod`/`go.sum`). Tahap 1 berisi migration sembilan tabel dan integration test skema. Bagian domain saat ini mencakup model bersama, operasi rupiah aman, tanggal bisnis, serta jadwal/periode berjalan. Perhitungan denda, alokasi pembayaran, dan pelunasan menjadi bagian anggota lain. Belum ada server HTTP, login pengurus, atau UI; akun MySQL berbeda dari akun pengurus aplikasi.

Sumber aturan adalah [RANCANGAN_SISTEM_KOPERASI.md](RANCANGAN_SISTEM_KOPERASI.md), khususnya [KD-01–KD-12](RANCANGAN_SISTEM_KOPERASI.md#22-keputusan-desain-untuk-ketentuan-yang-belum-dirinci-topik), arsitektur Bagian 8, skema Bagian 9, dan urutan tahap Bagian 14. Kebijakan tersebut perlu disetujui pengurus/dosen sebelum transaksi riil; README tidak membuat versi aturan lain.

## Tahap 2: fondasi bersama dan jadwal

Seluruh kode bagian ini berada dalam **satu package `koperasi/internal/domain`**, tanpa SQL, HTTP, pembacaan `.env`, atau waktu hari ini tersembunyi. Untuk memeriksanya, setiap anggota cukup memasang Go sesuai `go.mod` lalu menjalankan:

```sh
go test ./internal/domain
```

Tidak perlu MySQL atau kredensial laptop pemilik proyek untuk menjalankan unit test domain. Jangan mengubah migration atau dependency saat menambah perhitungan murni.

### Kontrak yang dipakai bersama

| Berkas | Isi/kontrak |
| --- | --- |
| `member.go` | `Member` dan `MemberStatus`, sesuai data anggota pada rancangan. |
| `saving.go` | `Saving`, jenis simpanan/transaksi, nominal pokok dan wajib tetap. |
| `loan.go` | `Loan`, `Installment`, status pinjaman, batas tenor dan konstanta tarif. |
| `payment.go` | `Payment`, `PaymentAllocation`, jenis pembayaran, komponen, tarif denda harian. |
| `date.go` | `ParseBusinessDate(string)` dan `NormalizeBusinessDate(time.Time)`, keduanya menghasilkan `(time.Time, error)`. |
| `money.go` | `ParseRupiah(string)` serta `AddMoney`, `SubtractMoney`, `MultiplyMoney`, `RoundHalfUp`; hasil `(int64, error)`. |
| `schedule.go` | `GenerateSchedule(principal int64, tenorMonths int, disbursementDate time.Time) ([]Installment, error)` dan `CurrentPeriod(disbursementDate time.Time, tenorMonths int, asOf time.Time) (int, error)`. |

Nominal dan ID memakai `int64`, tenor/nomor angsuran memakai `int`. Field nullable mengikuti skema dengan pointer, bukan nol yang dianggap NULL. Struct adalah data/snapshot, bukan pengganti validasi proses pada service. Tidak ada tag ORM atau query database pada model.

`InstallmentBalance` pada `loan.go` memuat satu `Installment` dan `PrincipalPaid`, `InterestPaid`, `PenaltyPaid`. Ketiganya merupakan jumlah pembayaran **sampai tanggal laporan/perhitungan yang diminta**, bukan keseluruhan histori masa depan dan bukan kolom saldo tambahan di database. Anggota denda/alokasi/pelunasan memakai tipe ini, tidak membuat versi model sendiri.

### Aturan penggunaan fungsi

- `ParseRupiah` menerima digit ASCII rupiah positif, misalnya `"100000"`; bukan teks `"Rp100.000"`, pecahan, tanda plus/minus, atau spasi. Nol bukan nominal transaksi yang sah. Operasi aritmetika menerima saldo nol tetapi menolak operand negatif, hasil pengurangan negatif, dan overflow.
- `RoundHalfUp(amount, divisor)` membulatkan pembagian bilangan nonnegatif dengan divisor positif. Jasa bulanan menggunakan `MonthlyInterestDivisor` (100, setara 1%); administrasi menggunakan `AdministrationDivisor` (50, setara 2%). Ini konstanta kebijakan, bukan tarif yang dapat dipilih pengurus.
- Tanggal bisnis menggunakan komponen tahun/bulan/hari. Normalisasi mempertahankan tanggal yang diberikan, mengabaikan jam/offset, dan menghasilkan UTC tengah malam **hanya sebagai pembawa tanggal**, bukan mengonversi instant. Jika input berasal dari instant riil, pemanggil harus mengambil tanggal Asia/Jakarta terlebih dahulu. Jangan memanggil `.In(...)` pada pembawa tanggal ini untuk mengubah tanggal bisnis.
- Tanggal mengikuti rentang MySQL DATE 1000–9999. `ParseBusinessDate` menerima format tepat `YYYY-MM-DD`; tidak ada pemanggilan `time.Now()` atau ketergantungan timezone laptop.
- Jadwal yang dihasilkan belum disimpan: `ID` dan `LoanID` nol. Service/repository kelak menghubungkannya dengan pinjaman saat pencairan. Jadwal tidak mengubah data input atau menyimpan transaksi.
- `GenerateSchedule` menggunakan jasa flat dari pokok awal, sisa pembagian pokok pada angsuran terakhir, dan tanggal 10 mulai bulan berikutnya. Nilai pokok+jasa seluruh tenor harus muat dalam `int64`; jika tidak, fungsi mengembalikan error tanpa jadwal parsial.
- Baris yang pokok dan jasanya sama-sama nol ditutup pada tanggal pencairan melalui `ClosedOn`, supaya kelak tidak menghasilkan denda. Baris yang memiliki kewajiban belum ditutup.
- `CurrentPeriod` mengembalikan nomor angsuran mulai 1: tanggal jatuh tempo masih periode yang sama, hari berikutnya masuk periode berikutnya. Sebelum pencairan atau setelah jatuh tempo terakhir menghasilkan `ErrOutsideSchedule`; tidak ada periode jasa tambahan setelah tenor selesai. Pembayaran periode tersebut tidak mengubah penentuan kalender.
- Gunakan `errors.Is` untuk `ErrInvalidAmount`, `ErrOverflow`, `ErrInvalidDivisor`, `ErrInvalidDate`, `ErrInvalidTenor`, dan `ErrOutsideSchedule`; jangan memeriksa teks error.

### Handoff untuk lima anggota

| Pemilik | Berkas yang menjadi tanggung jawab | Batas tugas |
| --- | --- | --- |
| Anggota 1, fondasi/integrator | Model, `money.go`, `date.go`, serta testnya | Menjaga kontrak bersama dan mengintegrasikan perubahan. |
| Anggota 2, jadwal | `schedule.go`, `schedule_test.go` | Jadwal dan periode berjalan. |
| Anggota 3, berikutnya | `penalty.go`, `penalty_test.go` | Denda dari tanggal/snapshot, termasuk pembayaran sebagian dan penghentian setelah lunas. |
| Anggota 4, berikutnya | `allocation.go`, `allocation_test.go` | Alokasi global denda → jasa → pokok, tertua dahulu per komponen. |
| Anggota 5, berikutnya | `settlement.go`, `settlement_test.go` | Pelunasan sesuai KD-06–KD-09, memakai hasil denda/alokasi dan periode berjalan yang sama. |

Bagian anggota 1–2 dikerjakan pada perubahan ini; tiga bagian berikutnya belum diimplementasikan dan tidak memiliki fungsi placeholder. Sebelum mengubah model/signature bersama, sepakati perubahan dengan integrator. Anggota 3–5 mengembangkan berkas terpisah dari basis fondasi yang sama; integrasi pelunasan menunggu kontrak denda/alokasi stabil. Masing-masing menyertakan test perilaku bagiannya, lalu integrator menjalankan `go test ./internal/domain` setelah penggabungan. Jangan menandai seluruh Tahap 2 selesai hanya karena test fondasi/jadwal lulus.

Verifikasi bagian anggota 1–2: `go test -count=1 -v ./internal/domain` lulus, dan smoke pemakaian API menghubungkan parsing rupiah/tanggal, pembuatan jadwal, penjumlahan aman, serta pemilihan periode berjalan. Test MySQL tidak diperlukan untuk perubahan domain ini dan tidak dijalankan ulang.

## Prasyarat dan konfigurasi

- Go **1.26.0 atau lebih baru** yang kompatibel dengan versi dependency terkunci (`go.mod`).
- **MySQL 8.4 LTS**, InnoDB, utf8mb4, strict SQL mode. MariaDB bukan pengganti yang dianggap setara.
- Client `mysql` di PATH; `mysqldump` untuk backup. Administrator MySQL untuk setup database dan akun.

Dari root proyek, di PowerShell:

```powershell
go mod download
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
$env:APP_ADDR = '127.0.0.1:8080'
$env:COOKIE_SECURE = 'false'
```

`.env` **tidak dimuat otomatis** oleh Go/test. Atur environment proses secara eksplisit, atau gunakan runner PowerShell di bawah yang hanya memuat TEST_DATABASE_DSN untuk proses test dan memulihkan environment setelahnya. `COOKIE_SECURE=false` hanya untuk HTTP localhost; gunakan `true` pada HTTPS ketika aplikasi tersedia.

### Apa itu DATABASE_DSN?

**DSN (Data Source Name) adalah teks konfigurasi koneksi ke MySQL**, bukan akun baru atau alamat website. Isinya username MySQL, password akun tersebut, alamat server, port, nama database, dan opsi koneksi.

Bentuk environment DSN berikut adalah **template**, bukan kredensial yang berlaku. `<akun_aplikasi>`, `<akun_test>`, dan `<password_lokal>` adalah penanda yang harus diganti dengan akun MySQL yang benar-benar ada; jangan menuliskan tanda `<...>` secara harfiah:

```text
DATABASE_DSN=<akun_aplikasi>:<password_lokal>@tcp(127.0.0.1:3306)/koperasi?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4
TEST_DATABASE_DSN=<akun_test>:<password_lokal>@tcp(127.0.0.1:3306)/koperasi_test?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4
```

| Bagian DSN | Arti |
| --- | --- |
| `<akun_aplikasi>` | Username MySQL yang digunakan program, misalnya `koperasi_app`; bukan username Windows atau login pengurus. |
| `<password_lokal>` | Password akun MySQL di sebelah kirinya, bukan password laptop. |
| `127.0.0.1` | Server MySQL pada laptop sendiri. |
| `3306` | Port koneksi MySQL. |
| `/koperasi` | Database pengembangan yang dipakai program. |
| `/koperasi_test` | Database terpisah khusus pengujian, bukan tempat data pengembangan. |
| Bagian setelah `?` | Opsi driver, misalnya pembacaan tanggal, timezone, dan charset. |

`DATABASE_DSN` disiapkan untuk aplikasi pada tahap berikutnya; test tahap 1 memakai `TEST_DATABASE_DSN`. Pada laptop yang sudah disetup, username dan password acak sudah diisi di `.env`: **tidak perlu membuat password sendiri atau mengganti nilainya dengan contoh README**. Saat teman melakukan clone di laptop lain, mereka harus menyiapkan akun dan kredensial lokal sendiri karena `.env` tidak ikut Git.

Di PowerShell, gunakan `$env:DATABASE_DSN = '...'` dan `$env:TEST_DATABASE_DSN = '...'` dengan DSN lokal Anda. Hindari menyimpan rahasia dalam riwayat terminal bersama, screenshot, log, atau Git. `loc=UTC` mengatur parsing driver; `time_zone` mengatur sesi MySQL setiap koneksi. Audit memakai UTC `DATETIME(6)`; tanggal bisnis Asia/Jakarta tetap `DATE`/`YYYY-MM-DD`, bukan instant yang digeser zona waktu.

## Setup database dan akun

Administrator membuat **dua database terpisah**:

```sql
CREATE DATABASE koperasi CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE DATABASE koperasi_test CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
```

### Mengapa ada akun selain root?

`root` adalah akun administrator MySQL yang dapat mengelola seluruh database. Secara teknis program juga bisa memakai root jika diizinkan server, tetapi proyek ini memakai akun terbatas agar kesalahan aplikasi/test tidak mendapat akses administrator.

Kamu tetap dapat login ke MySQL atau phpMyAdmin memakai root seperti biasa. Akun tambahan tidak mengganti root, tidak membuat user Windows, dan bukan akun login pengurus koperasi. phpMyAdmin hanyalah salah satu client MySQL; terminal dan program Go juga bisa terhubung langsung tanpa membuka atau login ke phpMyAdmin terlebih dahulu.

| Akun MySQL | Dipakai untuk | Hak minimum untuk pekerjaan ini |
| --- | --- | --- |
| `root` | Administrasi manual: membuat database, akun, dan memberikan hak akses. | Akses administrator; tidak dipakai aplikasi atau test. |
| `akun_migrasi` | Membuat atau memperbarui struktur tabel. | `CREATE`, `ALTER`, `INSERT`, `INDEX`, `REFERENCES` pada `koperasi.*`; MySQL juga memerlukan INSERT untuk ALTER TABLE. Tambah hak hanya bila migration baru membutuhkannya. |
| `koperasi_app` | Koneksi aplikasi melalui DATABASE_DSN. | `SELECT`, `INSERT`, `UPDATE`, `DELETE` pada `koperasi.*`, tanpa DDL atau akses database test. Hak DELETE diperlukan kelak untuk sesi, bukan izin menghapus histori keuangan lewat aplikasi. |
| `koperasi_test` | Menjalankan integration test melalui TEST_DATABASE_DSN. | `CREATE`, `ALTER`, `INDEX`, `REFERENCES`, `DROP`, `SELECT`, `INSERT`, `UPDATE`, `DELETE` hanya pada `koperasi_test.*`. Tidak boleh mengakses database pengembangan/riil. |

Administrator membuat akun-akun tersebut dengan password unik melalui saluran administratif lokal, bukan berkas yang dilacak. Nama akun tidak otomatis tercipta hanya karena ditulis di DSN. Setup laptop awal sudah membuat ketiganya, membatasi koneksi akun proyek ke `127.0.0.1`, dan menyimpan password aplikasi/test di `.env` serta password migration di `.env.migration.cnf`.

**Bagaimana setup dapat mengakses MySQL?** Pada saat provisioning laptop awal, perintah terminal lokal berhasil terhubung ke `127.0.0.1:3306` sebagai root tanpa password. Koneksi administrator itulah yang digunakan untuk membuat database dan akun terbatas melalui CREATE USER/GRANT; bukan aplikasi yang melewati login atau membuat akun secara otomatis. Akun/password root tidak diubah. Kondisi ini adalah catatan setup laptop awal, bukan asumsi kredensial untuk semua laptop.

Root tanpa password merupakan risiko keamanan. Jika akan diubah, periksa dahulu dampaknya terhadap phpMyAdmin dan proyek lain; jangan mengubah kredensial root diam-diam saat menjalankan test.

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

Tanpa environment test, hasil **SKIP bukan bukti lulus**. Fondasi database telah diverifikasi dengan Go 1.26.6 dan MySQL 8.4.3 terisolasi: `go mod verify` lulus; kedua migration berhasil dengan akun migration berhak terbatas; TestSchema lulus dengan akun test terpisah. Penolakan database test tidak kosong dan DSN database aplikasi juga diperiksa tanpa merusak data. Unit test domain dijalankan terpisah dengan `go test ./internal/domain`; server aplikasi belum diimplementasikan.

### Menjalankan tahap 1 di Windows

Setelah database dan akun lokal disiapkan, pastikan MySQL Laragon berjalan dan jalankan dari root proyek:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-stage1.ps1
```

Runner membaca TEST_DATABASE_DSN dari `.env` lokal dan menjalankan test sungguhan dengan `-count=1`, bukan hasil cache. Bypass execution policy hanya berlaku untuk proses PowerShell tersebut; tidak mengubah policy Windows secara permanen. Test memakai `koperasi_test`, sedangkan sembilan tabel pengembangan tetap berada di `koperasi`. Tahap 1 belum mempunyai server HTTP atau halaman yang bisa dibuka melalui localhost:8080.

Pada laptop yang telah diprovision, kredensial aplikasi/test tersimpan di `.env`, dan kredensial client migration tersimpan di `.env.migration.cnf`. Keduanya diabaikan Git dan tidak ikut clone rekan. Jangan menyalin `.env.example` di atas konfigurasi yang sudah diisi. Migration 001/002 sudah diterapkan saat setup lokal; jangan menjalankannya ulang pada database yang sudah berisi tabel.

Untuk migration baru kelak, client dapat menggunakan `mysql --defaults-file=.env.migration.cnf` dengan berkas migration baru melalui stdin CMD/Bash. Simpan file kredensial lokal tersebut, jangan membagikannya atau menaruh password pada command line.

## Backup dan pemulihan

Cadangkan setiap hari dan sebelum perubahan skema menggunakan `mysqldump --single-transaction` dengan akun backup yang sesuai; jangan ubah skema selama dump. Simpan backup berakses terbatas pada media/perangkat berbeda, bukan Git. Uji restore menggunakan client `mysql` ke database pemulihan terpisah yang disiapkan administrator; gunakan stdin CMD/Bash seperti migration. Periksa tabel, constraint, dan rekonsiliasi sebelum pemakaian. Jangan menimpa database riil atau menyalin direktori data MySQL aktif sebagai backup.

## Berbagi dan bekerja bersama

Lacak `go.mod`, **`go.sum`**, `.env.example`, `.gitignore`, README, **root `AGENTS.md`**, rancangan, serta `migrations/` termasuk test. Jangan bagikan `.env`, DSN/rahasia, dump, atau konfigurasi agent lokal. Gunakan remote/identitas Git yang benar-benar diberikan tim; proyek ini tidak menetapkan remote, akun, commit, atau push otomatis.

Root [AGENTS.md](AGENTS.md) adalah pedoman portabel lintas coding tool. Konfigurasi OMP lokal ada pada `.omp/agents/{koperasi-domain,koperasi-database,koperasi-web,koperasi-review}.md`; seluruh `/.omp/` diabaikan Git sehingga **tidak ikut clone rekan**. Definisi tersebut memakai model aktif yang diwarisi, tanpa pengaturan model/API tersendiri. Gunakan `/agents` di OMP untuk melihat definisinya. Subagent belum tentu otomatis mewarisi root AGENTS; prompt/definisi lokal harus secara eksplisit meminta membacanya beserta bagian rancangan terkait.

Bagi pekerjaan menjadi vertical slice kecil sesuai tahap, tentukan pemilik berkas sebelum mengedit, hormati perubahan rekan, lalu jalankan verifikasi sempit setelah integrasi. Jangan menganggap permintaan satu tahap sebagai izin membangun semua tahap berikutnya.
