param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

# Pone la misma versión x.y.z en los tres archivos que la declaran.
# Conserva la codificación de cada archivo (install.ps1 lleva BOM).

$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "Usa una versión con formato x.y.z; se recibió '$Version'."
}
$projectRoot = Split-Path -Parent $PSScriptRoot
$targets = @(
    @{ Path = 'version.go'; Pattern = '(appVersion\s*=\s*")[^"]+(")' },
    @{ Path = 'build\config.yml'; Pattern = '(?m)^(\s{2}version:\s*")[^"]+(")' },
    @{ Path = 'scripts\install.ps1'; Pattern = "(DisplayVersion\s+-Value\s+')[^']+(')" }
)

foreach ($target in $targets) {
    $path = Join-Path $projectRoot $target.Path
    $bytes = [IO.File]::ReadAllBytes($path)
    $hasBom = $bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
    $encoding = New-Object System.Text.UTF8Encoding($hasBom)
    $text = [IO.File]::ReadAllText($path, $encoding)
    if (-not [regex]::IsMatch($text, $target.Pattern)) {
        throw "No se encontró la versión en $($target.Path)."
    }
    $updated = [regex]::Replace($text, $target.Pattern, "`${1}$Version`${2}")
    [IO.File]::WriteAllText($path, $updated, $encoding)
    Write-Output "$($target.Path): $Version"
}
