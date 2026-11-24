# Koperasi Simpan Pinjam

Proyek Sistem Informasi **Koperasi Karyawan Sejahtera** berbasis Go, MySQL 8.4 LTS (InnoDB), HTML Template, dan Bootstrap lokal. Fitur utama telah diuji dengan data fiktif pada MySQL 8.4.3. Lihat [hasil audit dan checklist uji](PANDUAN_UJI_KOPERASI.md) untuk skenario dan hasil yang diharapkan.

Module Go menggunakan nama `koperasi`. Sesuai pedoman arsitektur, aplikasi mengutamakan *Go Standard Library* dengan dependensi minimal yang terkunci di `go.mod`: driver `github.com/go-sql-driver/mysql` dan `golang.org/x/crypto/bcrypt`.

Sumber aturan utama dan keputusan desain mengacu pada [RANCANGAN_SISTEM_KOPERASI.md](RANCANGAN_SISTEM_KOPERASI.md), khususnya aturan bisnis [KD-01–KD-12](RANCANGAN_SISTEM_KOPERASI.md#22-keputusan-desain-untuk-ketentuan-yang-belum-dirinci-topik), arsitektur Bagian 8, skema Bagian 9, serta urutan tahap Bagian 14.

---

## Status Ceklis Implementasi (Tahap 1 – 10)

| Tahap | Lingkup & Fitur | Status | Keterangan |
| :---: | :--- | :---: | :--- |
| **Tahap 1** | Module, Skema Database, dan Migrasi DDL | **[x] Selesai** | 9 tabel InnoDB, PK/FK, CHECK constraints, conditional unique index. |
| **Tahap 2** | Domain: Model dan Perhitungan Bisnis Murni | **[x] Selesai** | Rupiah integer overflow-safe, tanggal bisnis, jadwal flat 1%, denda harian, alokasi global, rumus pelunasan dipercepat. |
| **Tahap 3** | Konfigurasi Runtime & Connection Pool MySQL | **[x] Selesai** | Auto-load `.env`, pool strict UTC, timeout context, proteksi credential leak. |
| **Tahap 4** | Autentikasi Pengurus, Sesi, CSRF, & CLI Seed Admin | **[x] Selesai** | Password bcrypt, session cookie HTTP-only, CSRF guard, server graceful shutdown, CLI `seed-admin`. |
| **Tahap 5** | Manajemen Anggota dan Simpanan | **[x] Selesai** | Pendaftaran atomik simpanan pokok Rp500.000, simpanan wajib Rp100.000/bln, sukarela, penarikan, proses keluar anggota. |
| **Tahap 6** | Pengajuan, Persetujuan, & Pencairan Pinjaman | **[x] Selesai** | Batas maksimal 3× (pokok + wajib), tenor 6–24 bulan, status PENDING/APPROVED/ACTIVE, generate jadwal otomatis tgl 10. |
| **Tahap 7** | Pembayaran Angsuran & Denda Keterlambatan | **[x] Selesai** | Denda Rp2.000/hari, alokasi global **Denda → Jasa → Pokok**, pembayaran sebagian/penuh, penutupan angsuran, kuitansi. |
| **Tahap 8** | Pelunasan Dipercepat (*Early Settlement*) | **[x] Selesai** | Sisa pokok + jasa 1 bln berjalan + administrasi 2%, penyelesaian tunggakan lewat jatuh tempo, pembebasan jasa mendatang. |
| **Tahap 9** | Laporan Operasional & Dashboard | **[x] Selesai** | Kartu Pinjaman, Laporan Daftar Tunggakan, Laporan Penerimaan Jasa & Denda Bulanan, metrik riil Dashboard, ramah cetak browser. |
| **Tahap 10** | Verifikasi Menyeluruh & Panduan Uji Manual | **[x] Selesai** | Unit test, skema MySQL, alur transaksi utama, dan tampilan mobile diperiksa. |

---

## Prasyarat Lingkungan

- **Go**: Minimal versi `1.26.0` (kompatibel dengan versi pada `go.mod`).
- **MySQL**: Versi **8.4 LTS** dengan InnoDB dan utf8mb4.
- **Web Browser**: Chrome, Edge, Firefox, atau browser modern lainnya.

---

## Panduan Menjalankan Aplikasi (Setup & Run)

Gunakan akun MySQL `root` untuk instalasi lokal. Dari PowerShell di folder proyek, jalankan `mysql --user=root`. Jika root memiliki password, tambahkan `--password` agar klien memintanya.

Di prompt `mysql>`, buat dan pilih database aplikasi.

```sql
CREATE DATABASE IF NOT EXISTS koperasi CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
USE koperasi;
SHOW TABLES;
```

Jika database kosong, jalankan kedua migrasi dari prompt `mysql>` secara berurutan. Jika tabel sudah ada, periksa dulu migrasi yang sudah diterapkan.

```text
SOURCE migrations/001_schema.sql
SOURCE migrations/002_constraints_indexes.sql
```

Ketik `exit` untuk kembali ke PowerShell. Buat `.env` dari [`.env.example`](.env.example) jika belum ada. Template menggunakan root tanpa password, database `koperasi`, dan alamat web `127.0.0.1:8081`. Jika root memakai password, sesuaikan kedua DSN di `.env`.

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
```

Buat akun pengurus sekali dengan mengganti `GANTI_PASSWORD_UJI`, lalu jalankan server.

```powershell
go run ./cmd/seed-admin -username pengurus_uji -name 'Admin' '-password=GANTI_PASSWORD_UJI'
go run ./cmd/server
```

Buka [http://127.0.0.1:8081](http://127.0.0.1:8081) dan masuk sebagai `pengurus_uji` dengan password yang kamu pilih. Untuk menjalankan tes skema terpisah, ikuti [panduan pengujian](PANDUAN_UJI_KOPERASI.md#uji-otomatis).

---

## Panduan Uji Manual Fitur Satu per Satu (Step-by-Step)

Gunakan panduan berikut untuk memverifikasi seluruh proses bisnis koperasi dari awal hingga akhir:

### Skenario 1: Autentikasi Pengurus
1. Buka `http://127.0.0.1:8081/login`.
2. Coba masukkan password salah → sistem menampilkan pesan kesalahan.
3. Masuk sebagai `pengurus_uji` dengan password yang dibuat saat setup. Dashboard akan tampil.
4. Coba buka tab baru atau incognito tanpa login ke `http://127.0.0.1:8081/members` → sistem menolak dan mengarahkan kembali ke `/login` (proteksi sesi aktif).

### Skenario 2: Pendaftaran Anggota & Simpanan Pokok Atomik
1. Klik menu **Anggota** → klik **+ Tambah Anggota**.
2. Masukkan identitas:
   - No. Anggota: `ANG-001`
   - Nama: `Budi Santoso`
   - Telepon: `081234567890`
   - Alamat: `Jl. Melati No. 5`
   - Tanggal Bergabung: *(Pilih 5 Januari 2026)*
3. Klik **Daftarkan Anggota**.
4. **Verifikasi:** 
   - Anggota berstatus **AKTIF**.
   - Di tab Simpanan, otomatis tercatat setoran **Simpanan Pokok Rp500.000**.
   - Batas awal pinjaman anggota otomatis bernilai **Rp1.500.000** (3 × Rp500.000).

### Skenario 3: Setor Simpanan Wajib & Sukarela
1. Klik menu **Simpanan** → **+ Setor Simpanan**.
2. Pilih Anggota `Budi Santoso`, pilih jenis **Simpanan Wajib**, pilih periode Januari 2026 dan tanggal transaksi 10 Januari 2026.
3. Klik **Simpan Transaksi**.
   - **Verifikasi:** Nominal tersimpan tepat Rp100.000. Saldo simpanan wajib bertambah. Batas pinjaman naik menjadi **Rp1.800.000** (3 × (500rb + 100rb)).
4. Lakukan setor simpanan sekali lagi, pilih jenis **Simpanan Sukarela**, masukkan nominal bebas misalnya `Rp200.000`, pada 11 Januari 2026.
   - **Verifikasi:** Saldo sukarela bertambah Rp200.000. Batas pinjaman tetap Rp1.800.000 (sesuai aturan BR-05, simpanan sukarela tidak menambah batas pinjaman).

### Skenario 4: Penarikan Simpanan Sukarela
1. Klik menu **Simpanan** → **Tarik Sukarela**.
2. Pilih Anggota `Budi Santoso`.
3. Coba masukkan penarikan melebihi saldo sukarela (misal `Rp300.000`) → sistem menolak penarikan karena saldo tidak mencukupi.
4. Gunakan tanggal 12 Januari 2026 dan nominal sah `Rp50.000`, lalu klik Tarik.
5. **Verifikasi:** Sistem mencetak kuitansi penarikan dan saldo sukarela tersisa `Rp150.000`.

### Skenario 5: Pengajuan Pinjaman (Validasi Limit & Tenor)
1. Klik menu **Pinjaman** → **+ Ajukan Pinjaman**.
2. Pilih `Budi Santoso`:
   - Coba ajukan `Rp2.000.000` → sistem menolak karena melebihi batas maksimal Rp1.800.000.
   - Coba masukkan tenor `4` bulan atau `30` bulan → sistem menolak (tenor wajib 6–24 bulan).
3. Masukkan data sah: Pokok `Rp1.800.000`, Tenor `12` bulan, Tanggal Pengajuan 20 Januari 2026.
4. Klik **Kirim Pengajuan**.
5. **Verifikasi:** Pinjaman terbentuk dengan status **PENDING**.

### Skenario 6: Persetujuan, Pencairan, & Pembuatan Jadwal
1. Pada detail pinjaman, klik **Setujui Pinjaman** → Status berubah menjadi **APPROVED**.
2. Klik **Cairkan & Buat Jadwal**:
   - Masukkan tanggal pencairan, misal `2026-01-20`.
   - Klik **Cairkan Pinjaman Sekarang**.
3. **Verifikasi:**
   - Status pinjaman berubah menjadi **ACTIVE**.
   - Buka tombol **Lihat Jadwal Angsuran**:
     - Terbentuk 12 baris angsuran.
     - Angsuran ke-1 jatuh tempo `10 Februari 2026`, ke-2 `10 Maret 2026`, dst. tiap tanggal 10.
     - Pokok per bulan: `Rp150.000` (Rp1.800.000 / 12).
     - Jasa per bulan: `Rp18.000` (1% flat dari pokok awal Rp1.800.000). Total tagihan bulanan: `Rp168.000`.

### Skenario 7: Pembayaran Angsuran, Denda, & Alokasi Global
1. Di halaman detail pinjaman aktif, klik **+ Terima Pembayaran**.
2. **Uji Pembayaran Tepat Waktu**:
   - Tanggal bayar `2026-02-10`, nominal `Rp168.000`.
   - Klik Bayar → Kuitansi tercetak. Di jadwal, angsuran ke-1 tertutup (**LUNAS**).
3. **Uji Keterlambatan & Pembayaran Sebagian**:
   - Klik bayar lagi, pilih tanggal bayar lewat jatuh tempo: `2026-03-15`.
   - Sistem otomatis menghitung 5 hari terlambat (11–15 Maret) dengan denda `Rp10.000` (5 × Rp2.000).
   - Masukkan pembayaran sebagian, misal `Rp50.000`.
   - Klik Bayar.
   - **Verifikasi Alokasi Prioritas:** Kuitansi menunjukkan pembagian uang mengikuti aturan KD-04 (**Denda → Jasa → Pokok**):
     - Denda lunas: Rp10.000
     - Jasa terbayar Rp36.000, masing-masing Rp18.000 untuk Maret dan periode berjalan April.
     - Pokok terbayar sebagian Rp4.000. Sisa pokok angsuran Maret Rp146.000.

### Skenario 8: Pelunasan Dipercepat (*Early Settlement*)
1. Buka kembali detail pinjaman aktif, klik tombol **Pelunasan Dipercepat**.
2. Pilih tanggal 16 Maret 2026 dan hitung ulang pratinjau. Totalnya Rp1.678.000, terdiri dari:
   - **Tahap 1** melunasi tunggakan Rp148.000, termasuk tambahan denda Rp2.000.
   - **Tahap 2** melunasi sisa pokok Rp1.500.000 dan administrasi Rp30.000. Jasa April sudah dibayar.
   - Jasa bulan-bulan selanjutnya yang dibebaskan Rp162.000.
3. Klik tombol **Terima Uang & Lunasi Pinjaman**.
4. **Verifikasi:**
   - Kuitansi pelunasan dipercepat tercetak dengan rincian biaya.
   - Status pinjaman otomatis menjadi **SETTLED (LUNAS)**.
   - Jasa mendatang tidak dicatat sebagai pendapatan.

### Skenario 9: Pemeriksaan Laporan & Dashboard
1. Klik menu **Laporan** (`/reports/monthly`):
   - Menampilkan pendapatan jasa, denda keterlambatan, biaya administrasi, pengembalian pokok, dan total arus kas masuk bulan terpilih.
2. Klik tab **Daftar Tunggakan** (`/reports/arrears`):
   - Menampilkan daftar anggota yang menunggak beserta hari keterlambatan dan rincian nominal tagihan.
3. Di detail pinjaman anggota mana saja, klik **Kartu Pinjaman** (`/reports/loans/{id}`):
   - Menampilkan kartu pinjaman lengkap yang dapat dicetak langsung via tombol browser print (`Ctrl + P`).
4. Buka **Dashboard** (`/dashboard`):
   - Seluruh kartu statistik (Total Anggota, Total Simpanan, Pinjaman Aktif, Tunggakan, Pendapatan Bulan Ini) menampilkan angka riil hasil transaksi.

### Skenario 10: Proses Keluar Anggota
1. Buka menu **Anggota** → klik `Budi Santoso` → klik **Proses Keluar Anggota**.
2. Karena pinjaman sudah **LUNAS (SETTLED)**, sistem mengizinkan proses keluar.
3. Form menampilkan rincian seluruh saldo simpanan pokok, wajib, dan sukarela yang akan dikembalikan.
4. Pilih 17 Maret 2026 dan klik **Konfirmasi Keluar & Kembalikan Simpanan**.
5. **Verifikasi:** Seluruh saldo simpanan menjadi Rp0, status anggota menjadi **KELUAR (EXITED)**, dan seluruh rekam jejak transaksi historis tetap tersimpan di database.

---

## Verifikasi Otomatis (Automated Tests)

Untuk memverifikasi seluruh unit test domain, middleware, service, dan handler secara otomatis:

```powershell
go test -count=1 -v ./...
```
Untuk tes skema dan koneksi, set `TEST_DATABASE_DSN` ke database kosong khusus uji berakhiran `_test`. Tanpa variabel ini, kedua tes tersebut berstatus `SKIP`. Lihat [setup lengkap](PANDUAN_UJI_KOPERASI.md#menyiapkan-lingkungan-uji).
