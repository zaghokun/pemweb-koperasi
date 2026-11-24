# Hasil audit dan panduan uji Koperasi Karyawan Sejahtera

Audit awal 24 November 2026, commit `d928901`, Go 1.26.6 dan MySQL 8.4.3 dalam mode ketat. Acuan bisnisnya adalah Topik 2 dan [rancangan proyek](RANCANGAN_SISTEM_KOPERASI.md#2-sumber-aturan-dan-keputusan-desain).

## Hasil uji ulang setelah perbaikan

Alur utama lulus dari pendaftaran, simpanan, pencairan, pembayaran sebagian, dan pelunasan hingga keluar anggota di MySQL 8.4.3 dengan data fiktif. Uji lain memastikan pembayaran terakhir menutup pinjaman tanpa administrasi, pengiriman ulang pelunasan mengarah ke kuitansi yang sama, tanggal transaksi mundur ditolak, dan laporan historis mengikuti tanggal pilihannya. Pengujian skema juga menjalankan regresi transaksi dari service aplikasi.

Menu HP bisa dibuka dan ditutup. Pada layar uji 320 piksel, daftar anggota, simpanan, pinjaman, detail pinjaman, kartu, dan kedua laporan pas dengan lebar halaman. Tabel lebar dapat digeser di dalam area tabel. Kartu pinjaman juga diperiksa pada 390, 768, dan 1.280 piksel. Tautan halaman berikutnya mempertahankan filter daftar.

## Kondisi saat audit awal

Pada commit awal, alur web dari pendaftaran sampai pelunasan gagal. Pengujian otomatis yang tersedia saat itu lulus, termasuk tes skema dan koneksi ke MySQL 8.4.3. Untuk memeriksa fitur di balik penghalang awal, audit memakai salinan sementara yang menyesuaikan pembacaan `DATE` dan mempertahankan metadata pinjaman saat perubahan status. Satu uji pelunasan mengirim kunci 32 karakter melalui HTTP.

### Perbandingan awal dengan Topik 2

| Lingkup | Hasil pengujian |
| --- | --- |
| 1. Anggota dan simpanan | Pendaftaran menyimpan anggota aktif dan satu setoran pokok Rp500.000. Build asli kemudian menampilkan HTTP 500 pada detail, daftar anggota, dan riwayat simpanan. Setoran wajib melalui web ikut gagal. Setelah penyesuaian tanggal, pencarian, ubah identitas, setoran wajib Rp100.000, setoran sukarela, penarikan, dan pengembalian tiga jenis saldo saat keluar berhasil. |
| 2. Pengajuan dan pencairan | Setelah penyesuaian tanggal, batas Rp1.800.000 dan tenor 6–24 berfungsi pada kasus yang diuji, pengajuan kedua ditolak, dan persetujuan berhasil. Pencairan gagal karena perubahan status mengosongkan `approved_by`. |
| 3. Jadwal angsuran | Unit test jadwal lulus. Setelah metadata dipertahankan di salinan diagnosis, pencairan Rp1.800.000 menghasilkan 12 angsuran mulai 10 Februari 2026, dengan pokok Rp150.000 dan jasa flat Rp18.000 per bulan. |
| 4. Pembayaran angsuran | Salinan diagnosis menghasilkan pembayaran tepat waktu, denda per angsuran, alokasi sebagian global dan kuitansi sesuai hitungan. Pembayaran akhir gagal dan ter-rollback pada salinan yang hanya menyesuaikan tanggal. Status menjadi lunas setelah metadata pinjaman dipertahankan. Tanggal pembayaran yang mundur masih diterima. |
| 5. Pelunasan dipercepat | Salinan diagnosis menghasilkan pratinjau Rp1.678.000 dan pembebasan jasa Rp162.000. Pengiriman formulir gagal dengan MySQL 1406. Kuitansi dan penutupan pinjaman baru berhasil pada uji HTTP dengan kunci 32 karakter dan metadata yang dipertahankan. Pengiriman ulang gagal mengembalikan kuitansi yang sama. |
| 6. Kartu pinjaman, tunggakan, dan laporan bulanan | Laporan Maret pada build asli cocok dengan transaksi yang disiapkan dalam audit. Kartu menampilkan histori di salinan diagnosis, tetapi sisa pokok dan sisa kewajiban belum ditampilkan terpisah, pilihan posisi tanggal belum tersedia, dan jasa pada judul bisa berbeda Rp1 dari jadwal. Daftar tunggakan gagal pada build asli saat ada pinjaman aktif yang menunggak. Salinan diagnosis masih menghilangkan tunggakan historis setelah pinjaman dilunasi. |

Login, pesan untuk password salah, logout, pembatasan rute tanpa sesi dan penolakan POST tanpa token CSRF terverifikasi. Pada lebar browser 313 piksel, navigasi membuat halaman laporan melebar hingga 542 piksel. Hasil cetak laporan bulanan terbaca pada dua halaman PDF dan masih menampilkan pemilih bulan dan tahun. Cetak kartu, jadwal, dan kuitansi belum diperiksa.

Pendaftaran pada build asli berhasil di-rollback ketika fixture database memaksa setoran pokok gagal. Pada salinan diagnosis, dua akun yang masing-masing mengajukan penarikan Rp80.000 dari saldo Rp100.000 hanya berhasil sekali, dengan sisa Rp20.000. Dua pengajuan dan dua pencairan bersamaan masing-masing juga berhasil sekali.

Dengan 250 anggota fiktif, salinan diagnosis bisa mencari anggota lama dan memuat halaman kedua lewat `?page=2`. Halaman pertama tidak menyediakan tautan untuk berpindah. Backup dipulihkan ke database lain dalam instance uji. Jumlah anggota, saldo per jenis, penerimaan bulanan per komponen dan jumlah constraint sama setelah pemulihan.

### Temuan audit awal

| Temuan | Cara mereproduksi dan hasil audit |
| --- | --- |
| Pembacaan tanggal menghentikan alur web | Daftarkan anggota, lalu buka detailnya. Driver dengan `parseTime=true` mengembalikan `DATE` sebagai `time.Time`. Repository memindainya melalui string dan memanggil parser yang hanya menerima `YYYY-MM-DD`. Anggota dan setoran pokok sudah tersimpan saat detail menampilkan HTTP 500. Buka juga daftar anggota, daftar simpanan dan kuitansi. [Pool MySQL](internal/database/mysql.go#L52), [pemetaan anggota](internal/repository/members.go#L192). |
| Pencairan dan transisi lunas gagal | Setujui pinjaman lalu cairkan. Setelah masalah tanggal dilewati, MySQL menolak pencairan dengan 3819 dan tidak menyimpan jadwal. Pencairan mengosongkan pengurus yang menyetujui. Pembayaran akhir dan pelunasan dipercepat juga mengosongkan metadata yang wajib dipertahankan saat status menjadi `SETTLED`. [Pencairan](internal/service/loans.go#L266), [pembayaran akhir](internal/service/payments.go#L387), [pelunasan](internal/service/settlements.go#L263), [constraint](migrations/001_schema.sql#L123). |
| Konfirmasi pelunasan gagal | Kirim pratinjau yang masih berlaku. Formulir menghasilkan token dengan awalan `settle-` dan ID pinjaman, sehingga panjangnya melebihi `CHAR(32)`. MySQL menolak insert dan pinjaman tetap aktif. Setelah pelunasan berhasil di salinan diagnosis, pengiriman ulang ditolak karena status sudah lunas. Harapannya, pengiriman ulang yang identik membuka kuitansi lama tanpa pembayaran tambahan. [Pembuatan kunci](internal/handler/settlements.go#L41), [kolom database](migrations/001_schema.sql#L179), [urutan cek pelunasan](internal/service/settlements.go#L137). |
| Tanggal transaksi melanggar KD-12 | Salinan diagnosis menerima setoran tahun 2099, setoran sebelum transaksi terakhir, dan pembayaran 10 September setelah pembayaran 11 September sudah tercatat. Beberapa halaman laporan dan pelunasan juga mengambil tanggal default dari UTC. Uji batas pergantian hari menggunakan Asia/Jakarta. [Aturan KD-12](RANCANGAN_SISTEM_KOPERASI.md#22-keputusan-desain-untuk-ketentuan-yang-belum-dirinci-topik), [tanggal laporan](internal/handler/reports.go#L15). |
| Laporan historis dan pembulatan kartu | Buka laporan tunggakan bertanggal sebelum pelunasan, sesudah pelunasan dicatat. Pinjaman hilang karena query hanya membaca status `ACTIVE` terkini. Kartu tidak mengubah hasil saat tanggal pada URL diubah. Pada pinjaman Rp1.000.050, jadwal menyimpan jasa Rp10.001, sedangkan ringkasan kartu menulis Rp10.000/bln. [Query tunggakan](internal/repository/reports.go#L189), [service kartu](internal/service/reports.go#L171), [template kartu](web/templates/reports/loan_card.html). |
| Urutan lock berbeda dari rancangan | Pencairan, pembayaran, dan pelunasan mengunci pinjaman lebih dahulu. Rancangan mengharuskan anggota dikunci dahulu dan angsuran dikunci sesuai urutan. Ini temuan dari pembacaan kode. Uji dua penarikan bersamaan berhasil menjaga saldo pada skenario audit. Deadlock dan lock timeout belum disimulasikan. [Pola transaksi](RANCANGAN_SISTEM_KOPERASI.md#101-pola-transaksi-keuangan), [pencairan](internal/service/loans.go#L216), [pembayaran](internal/service/payments.go#L222), [pelunasan](internal/service/settlements.go#L137). |

README menyebut semua tahap telah diverifikasi, sedangkan AGENTS.md masih menjelaskan tahap awal. Contoh transaksi README juga mencampurkan tanggal pengajuan hari ini dengan tanggal pencairan lampau. Gunakan tanggal yang konsisten seperti di bawah. Untuk pembayaran 15 Maret, contoh README melewatkan jasa angsuran April yang sudah menjadi periode berjalan menurut KD-04 sampai KD-06. [README](README.md#panduan-uji-manual-fitur-satu-per-satu-step-by-step).

## Menyiapkan lingkungan uji

1. Gunakan PowerShell dari direktori proyek, Go minimal 1.26.0, dan MySQL 8.4 LTS dengan InnoDB. Periksa versi dan pastikan `@@sql_mode` berisi mode ketat, misalnya `STRICT_TRANS_TABLES`.

   ```powershell
   go version
   mysql --version
   mysql --user=root --password -e 'SELECT VERSION(), @@sql_mode'
   go mod download
   go mod verify
   ```

2. Buat dua database kosong khusus uji. Gunakan database web untuk skenario manual dan database berakhiran `_test` untuk test otomatis. Jalankan SQL berikut dengan akun administrasi MySQL lokal. Ganti kedua password contoh.

   ```sql
   CREATE DATABASE koperasi_web_uji CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
   CREATE DATABASE koperasi_skema_test CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;

   CREATE USER 'koperasi_web_uji'@'127.0.0.1' IDENTIFIED BY 'GANTI_PASSWORD_WEB';
   GRANT SELECT, INSERT, UPDATE, DELETE ON koperasi_web_uji.* TO 'koperasi_web_uji'@'127.0.0.1';

   CREATE USER 'koperasi_skema_test'@'127.0.0.1' IDENTIFIED BY 'GANTI_PASSWORD_TEST';
   GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, INDEX, REFERENCES
     ON koperasi_skema_test.* TO 'koperasi_skema_test'@'127.0.0.1';
   ```

3. Terapkan `001` dan `002` hanya pada `koperasi_web_uji` menggunakan akun migrasi yang berwenang pada database tersebut. Pada instance MySQL lokal khusus uji, contoh di bawah memakai `root` untuk menjalankan migrasi. Jika perintah pertama gagal, periksa pesan MySQL sebelum menjalankan migrasi kedua.

   ```powershell
   Get-Content -Raw migrations/001_schema.sql | mysql --user=root --password koperasi_web_uji
   Get-Content -Raw migrations/002_constraints_indexes.sql | mysql --user=root --password koperasi_web_uji
   ```

4. Isi environment terminal. Ganti password contoh dan sesuaikan port jika berbeda. `TEST_DATABASE_DSN` harus diteruskan sebagai environment variable karena perintah `go test` membacanya langsung.

   ```powershell
   $env:APP_ADDR = '127.0.0.1:8081'
   $env:COOKIE_SECURE = 'false'
   $env:DATABASE_DSN = 'koperasi_web_uji:GANTI_PASSWORD_WEB@tcp(127.0.0.1:3306)/koperasi_web_uji?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4'
   $env:TEST_DATABASE_DSN = 'koperasi_skema_test:GANTI_PASSWORD_TEST@tcp(127.0.0.1:3306)/koperasi_skema_test?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4'
   go test -count=1 -v ./...
   go vet ./...
   ```

   Pastikan `TestSchema` dan `TestOpen_LiveDatabaseIfConfigured` menunjukkan `PASS`. `SKIP` berarti pemeriksaan yang membutuhkan MySQL belum berjalan.

5. Buat akun fiktif dan jalankan server. Gunakan prompt untuk memasukkan password akun uji, lalu buka [http://127.0.0.1:8081](http://127.0.0.1:8081).

   ```powershell
   $passwordUjiKoperasi = Read-Host 'Password akun pengurus uji'
   go run ./cmd/seed-admin -username pengurus_uji -name 'Pengurus Fiktif' -password $passwordUjiKoperasi
   Remove-Variable passwordUjiKoperasi
   go run ./cmd/server
   ```

## Checklist alur utama di browser

Gunakan anggota fiktif `UJI-001`, nama `Anggota Uji`, alamat `Alamat Uji` dan telepon `080000000000`. Gunakan tanggal di bawah dan masukkan nominal sebagai digit, misalnya `1800000`. Pada setiap langkah, centang setelah hasil aktual sesuai. Jika satu langkah gagal, catat URL, pesan, dan apakah transaksi tercatat. Apabila pendaftaran mengarah ke halaman gagal, periksa keberadaan setoran pokok dengan kueri di bagian akhir sebelum mengirim ulang.

- [x] **Login.** Buka `/members` di jendela privat. Harus diarahkan ke login. Masukkan password salah dan pastikan pesan tampil. Masukkan password benar dan pastikan dashboard tampil.
- [x] **Pendaftaran.** Klik **Anggota → Tambah Anggota**. Gunakan tanggal masuk 5 Januari 2026. Setelah simpan, buka detail. Status aktif, riwayat memuat tepat satu setoran pokok Rp500.000, dan batas pinjaman Rp1.500.000. Coba nomor yang sama lagi dan cek tetap satu anggota. Cari nomor anggota dan ubah identitas. Nomor anggota, tanggal masuk, dan histori tetap sama.
- [x] **Wajib.** Dari detail anggota klik **Setor Simpanan**. Pilih wajib dengan periode Januari 2026 dan tanggal transaksi 10 Januari 2026. Kuitansi Rp100.000 dan batas pinjaman menjadi Rp1.800.000. Ulangi periode Januari untuk memastikan setoran kedua ditolak.
- [x] **Sukarela.** Setor Rp200.000 pada 11 Januari. Pastikan kuitansi Rp200.000 dan batas pinjaman tetap Rp1.800.000. Pada 12 Januari coba tarik Rp300.000, lalu Rp50.000. Permintaan pertama ditolak dan yang kedua menghasilkan kuitansi dengan saldo tersisa Rp150.000. Form penarikan biasa hanya menyediakan simpanan sukarela.
- [x] **Pengajuan.** Gunakan tanggal 20 Januari. Coba pokok Rp1.800.001, tenor 5 dan tenor 25 secara terpisah. Cek setiap pengajuan ditolak. Ajukan Rp1.800.000 dengan tenor 12 bulan dan cek status `PENDING`. Pengajuan kedua untuk anggota yang sama harus ditolak.
- [x] **Pencairan.** Setujui pinjaman dan pastikan status `APPROVED` tanpa jadwal. Cairkan pada 20 Januari dan cek status `ACTIVE`. Ulangi pencairan dan cek jumlah baris jadwal tetap 12. Saat pinjaman aktif, proses keluar anggota harus ditolak.
- [x] **Jadwal.** Cek 12 jatuh tempo dari 10 Februari 2026 sampai 10 Januari 2027. Tiap baris memuat pokok Rp150.000, jasa Rp18.000, total Rp168.000. Total pokok jadwal Rp1.800.000.
- [x] **Bayar tepat waktu.** Buka **Terima Pembayaran**. Ubah tanggal ke 10 Februari, tunggu pratinjau diperbarui, lalu bayar Rp168.000. Cek kuitansi berisi jasa Rp18.000, pokok Rp150.000, denda Rp0 dan angsuran pertama lunas. Muat ulang kuitansi dan cek pembayaran tetap satu.
- [x] **Bayar sebagian dan menunggak.** Ubah tanggal ke 15 Maret, lalu bayar Rp50.000. Cocokkan alokasi dengan tabel di bawah. Pada laporan tunggakan bertanggal 16 Maret, anggota ini memiliki satu tunggakan Rp148.000, terdiri dari pokok Rp146.000 dan sisa denda Rp2.000. Di kartu pinjaman, sisa pokok keseluruhan seharusnya Rp1.646.000.
- [x] **Pelunasan.** Dari detail pinjaman buka **Pelunasan Dipercepat**. Pada pinjaman historis, bila tombol membuka pesan bahwa tanggal di luar jadwal, buka URL `/loans/ID_PINJAMAN/settlement?date=2026-03-16` di server uji. Pratinjau harus menunjukkan penyelesaian tunggakan Rp148.000 dan sisa pokok setelah tahap itu Rp1.500.000. Jasa April sudah terbayar. Administrasi Rp30.000, total terima Rp1.678.000, jasa dibebaskan Rp162.000. Konfirmasi sekali, lalu cek kuitansi, status `SETTLED`, dan tidak ada sisa kewajiban.
- [x] **Tunggakan historis.** Setelah pelunasan, buka laporan tanggal 15 Maret. Anggota masih harus muncul dengan sisa tunggakan Rp146.000 setelah pembayaran sebagian hari itu. Pada posisi 16 Maret setelah pelunasan, anggota tidak muncul.
- [x] **Laporan bulanan dan kartu.** Pilih Maret 2026 pada menu **Laporan**. Untuk alur utama saja, jasa Rp36.000, denda Rp12.000, administrasi Rp30.000, pokok Rp1.650.000, total kas masuk Rp1.728.000. Jasa yang dibebaskan tidak menambah pendapatan. Kartu pinjaman harus memuat dua pembayaran biasa dan satu pelunasan, dengan tautan kuitansi. Coba `Ctrl+P` pada kartu, jadwal, laporan, dan kuitansi, lalu cek pratinjau halaman.
- [x] **Keluar anggota.** Pada 17 Maret buka **Keluar Anggota** dari detail anggota. Konfirmasi pengembalian pokok Rp500.000, wajib Rp100.000, dan sukarela Rp150.000, total Rp750.000. Status menjadi `EXITED`, saldo ketiga jenis nol, histori masih bisa dibuka, dan transaksi baru ditolak.
- [x] **Logout.** Klik **Keluar**. Membuka ulang URL kartu pinjaman di sesi tersebut harus mengarahkan ke login.

Alokasi dari pembayaran Rp50.000 pada 15 Maret

| Komponen | Angsuran | Alokasi |
| --- | --- | ---: |
| Denda 5 hari | Jatuh tempo 10 Maret | Rp10.000 |
| Jasa | Jatuh tempo 10 Maret | Rp18.000 |
| Jasa periode berjalan | Jatuh tempo 10 April | Rp18.000 |
| Pokok | Jatuh tempo 10 Maret | Rp4.000 |

## Checklist skenario tambahan

Buat anggota baru untuk skenario yang membutuhkan saldo atau histori berbeda.

- [ ] Bayar wajib untuk lima periode. Batas pinjaman menjadi Rp3.000.000. Coba tenor tepat 6 dan 24 bulan pada anggota dengan pengajuan terbuka yang sudah ditutup. Tolak pengajuan dan periksa alasan serta histori persetujuan jika sebelumnya sudah disetujui.
- [ ] Cairkan Rp1.000.000 tenor 6 bulan pada 31 Januari. Jadwal pertama 10 Februari. Lima tagihan Rp176.666 dan terakhir Rp176.670. Ulangi pencairan pada Desember untuk memeriksa pergantian tahun. Pada Rp1.000.050, jasa pada kartu dan jadwal harus sama-sama Rp10.001.
- [ ] Pada pinjaman baru, cek denda Rp0 saat jatuh tempo, Rp2.000 sehari kemudian dan Rp8.000 empat hari kemudian. Bila dua tagihan jatuh tempo 10 Februari dan 10 Maret dan keduanya belum dibayar, denda pada 13 Maret 2026 harus Rp68.000. Bayar Rp5.000 saat denda Rp8.000 dan pastikan jasa serta pokok belum berkurang.
- [ ] Coba nominal nol, negatif, pecahan, melebihi batas pembayaran dan di atas kapasitas `int64`. Coba tanggal transaksi mendatang, sebelum kewajiban dimulai, atau lebih awal daripada transaksi keuangan terakhir anggota. Harus ditolak tanpa tambahan baris transaksi.
- [ ] Catat pembayaran biasa untuk seluruh kewajiban tepat pada jatuh tempo terakhir. Pinjaman menjadi lunas, pembayaran masuk sekali dan administrasi 2% tidak dikenakan. Ulangi skenario pada pinjaman lain sesudah jatuh tempo terakhir, dengan denda yang berlaku.
- [ ] Uji pelunasan tanpa tunggakan. Dengan pinjaman Rp1.800.000 setelah pembayaran Februari, lunasi pada 5 Maret. Harapannya sisa pokok Rp1.650.000, jasa berjalan Rp18.000, administrasi Rp33.000, total Rp1.701.000 dan pembebasan jasa Rp180.000. Gunakan anggota terpisah dari alur utama.
- [ ] Buka pratinjau pelunasan di dua sesi. Catat pembayaran Rp1 di sesi kedua, lalu konfirmasi nominal lama di sesi pertama. Pratinjau lama harus ditolak. Setelah konfirmasi yang sah, pengiriman ulang formulir identik harus mengarah ke kuitansi lama.
- [ ] Gunakan dua akun pengurus dan dua jendela terpisah. Jalankan dua penarikan Rp80.000 dari saldo Rp100.000. Maksimal satu boleh berhasil dan saldo akhir Rp20.000. Ulangi dengan dua pengajuan untuk anggota yang sama dan dua konfirmasi pencairan pinjaman yang sama. Periksa jumlah pinjaman dan jadwal di database.
- [ ] Dengan lebih dari 20 anggota, periksa navigasi halaman berikutnya dan cari anggota lama berdasarkan nomor. Pada lebar sekitar 320 piksel, coba buka semua menu, gunakan keyboard, dan periksa tabel jadwal serta laporan.

## Pemeriksaan SQL baca-saja

Jalankan di `koperasi_web_uji` dengan akun baca. Ganti `UJI-001` atau `123` sesuai data fiktif yang dipakai.

```sql
SELECT m.member_number, m.status, s.saving_type,
       s.transaction_type, s.amount, s.period_month, s.transaction_date
FROM members AS m
LEFT JOIN savings AS s ON s.member_id = m.id
WHERE m.member_number = 'UJI-001'
ORDER BY s.transaction_date, s.id;

SELECT l.id, l.status, l.principal, l.approved_by, l.disbursed_by,
       COUNT(i.id) AS jumlah_angsuran, SUM(i.principal_due) AS pokok_jadwal
FROM loans AS l
LEFT JOIN installments AS i ON i.loan_id = l.id
WHERE l.id = 123
GROUP BY l.id, l.status, l.principal, l.approved_by, l.disbursed_by;

SELECT p.id, p.payment_date, p.payment_type, p.amount,
       i.installment_number, a.component_type, a.amount AS alokasi
FROM payments AS p
JOIN payment_allocations AS a ON a.payment_id = p.id
LEFT JOIN installments AS i ON i.id = a.installment_id
WHERE p.loan_id = 123
ORDER BY p.payment_date, p.id, a.id;
```

Untuk setiap pembayaran, jumlah kolom `alokasi` harus sama dengan nominal pada kuitansi. Setelah penolakan transaksi, ulangi kueri yang sama untuk memastikan tidak ada baris baru. Untuk pengujian audit, catat tanggal uji, versi commit, hasil yang diharapkan, hasil aktual, dan bukti kuitansi atau laporan per skenario.
