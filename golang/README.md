# Quran Images Generator - Go Implementation

Aplikasi Go untuk generate gambar halaman Al-Quran dari database PostgreSQL.

## Requirements

- Go 1.21 atau lebih baru
- PostgreSQL database dengan schema yang sesuai
- Font TTF di folder `../res/fonts/`

## Installation

1. Install dependencies:
```bash
cd golang
go mod download
```

2. Pastikan konfigurasi database di `config/database.yaml` sudah benar

3. Build aplikasi:
```bash
go build -o generate main.go
```

## Usage

```bash
./generate --pages [range] --width [width] --output [directory]
```

### Options

- `--pages, -p`: Nomor halaman atau range halaman yang akan diproses (contoh: `1..604` atau `50`)
- `--width, -w`: Lebar gambar dalam pixels (default: 1024)
- `--output, -o`: Direktori output untuk gambar yang dihasilkan (default: `images`)
- `--help, -h`: Menampilkan bantuan

### Examples

```bash
# Generate semua halaman dengan lebar 1280px
./generate --pages 1..604 --width 1280 --output ./images

# Generate halaman 50 dengan lebar 1024px
./generate --pages 50 --width 1024

# Generate halaman 51 dan 604 dengan lebar 1300px
./generate --pages 51 --width 1300
./generate --pages 604 --width 1300
```

## Database Schema

Aplikasi ini membutuhkan tabel-tabel berikut di database PostgreSQL:

- `glyph_page_line`
- `glyph`
- `glyph_type`
- `glyph_page_line_bbox`

Pastikan struktur database sesuai dengan schema yang digunakan oleh aplikasi Perl.

## Notes

- Aplikasi ini adalah migrasi dari aplikasi Perl di `script/generate.pl`
- Output gambar akan disimpan dalam format PNG
- Aplikasi akan mereset tabel `glyph_page_line_bbox` sebelum memproses halaman
- Font default yang digunakan adalah `QCF_BSML.TTF`
