$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$envPath = Join-Path $projectRoot '.env'

if (-not (Test-Path -LiteralPath $envPath -PathType Leaf)) {
    throw 'Buat .env lokal sesuai README sebelum menjalankan test.'
}

$entries = @(Get-Content -LiteralPath $envPath | Where-Object { $_ -match '^TEST_DATABASE_DSN=' })
if ($entries.Count -ne 1) {
    throw '.env harus memiliki tepat satu TEST_DATABASE_DSN.'
}
$testDsn = ($entries[0] -replace '^TEST_DATABASE_DSN=', '').Trim().Trim('"')
if ([string]::IsNullOrWhiteSpace($testDsn) -or $testDsn.Contains('CHANGE_ME')) {
    throw 'TEST_DATABASE_DSN belum diisi dengan konfigurasi test lokal.'
}

$previousDsn = $env:TEST_DATABASE_DSN
Push-Location $projectRoot
try {
    $env:TEST_DATABASE_DSN = $testDsn
    & go test -count=1 -v ./migrations
    if ($LASTEXITCODE -ne 0) {
        throw 'Test tahap 1 gagal; periksa pesan di atas. Jangan reset database berisi data.'
    }
}
finally {
    $env:TEST_DATABASE_DSN = $previousDsn
    Pop-Location
}
