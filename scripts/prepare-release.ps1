param(
    [Parameter(Mandatory = $true)]
    [string]$Version,
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$outputDirectory = Join-Path $projectRoot 'bin'
$releaseRepository = 'sergioc0702/musica-nativa'
$releaseExecutableName = 'musica-nativa-windows-amd64.exe'
$wailsBin = Join-Path $env:USERPROFILE 'go\bin'
$env:PATH = "$wailsBin;$env:PATH"
$wails = Join-Path $wailsBin 'wails3.exe'

if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "Usa una versión estable con formato x.y.z; se recibió '$Version'."
}
if (-not (Test-Path -LiteralPath $wails -PathType Leaf)) {
    throw "No se encontró Wails CLI: $wails"
}

$versionSource = Get-Content -LiteralPath (Join-Path $projectRoot 'version.go') -Raw
$versionMatch = [regex]::Match($versionSource, 'appVersion\s*=\s*"([^"]+)"')
$configSource = Get-Content -LiteralPath (Join-Path $projectRoot 'build\config.yml') -Raw
$configMatch = [regex]::Match($configSource, '(?m)^\s{2}version:\s*["'']?([^"''\r\n]+)["'']?\s*$')
$installerSource = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'install.ps1') -Raw
$installerMatch = [regex]::Match($installerSource, "DisplayVersion\s+-Value\s+'([^']+)'")
if (-not $versionMatch.Success -or -not $configMatch.Success -or -not $installerMatch.Success) {
    throw 'No se pudieron leer las tres versiones del proyecto.'
}
$projectVersion = $versionMatch.Groups[1].Value
if ($Version -ne $projectVersion -or $Version -ne $configMatch.Groups[1].Value -or $Version -ne $installerMatch.Groups[1].Value) {
    throw "La versión solicitada ($Version) debe coincidir con version.go, build/config.yml y scripts/install.ps1 (actual: $projectVersion)."
}

if (-not $SkipBuild) {
    Push-Location $projectRoot
    try {
        & $wails task package
        if ($LASTEXITCODE -ne 0) {
            throw "La compilación y el empaquetado terminaron con código $LASTEXITCODE."
        }
    } finally {
        Pop-Location
    }
}

$appExecutable = Join-Path $outputDirectory 'musica-nativa.exe'
if (-not (Test-Path -LiteralPath $appExecutable -PathType Leaf)) {
    throw "No se encontró el ejecutable de la app: $appExecutable"
}
$installer = Get-ChildItem -LiteralPath $outputDirectory -Filter 'Musica-Nativa-Setup-*.exe' -File |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1
if (-not $installer) {
    throw "No se encontró el instalador de primera instalación en $outputDirectory"
}

$releaseDirectory = Join-Path $outputDirectory (Join-Path 'releases' "v$Version")
New-Item -ItemType Directory -Path $releaseDirectory -Force | Out-Null
$appAsset = Join-Path $releaseDirectory $releaseExecutableName
$installerAsset = Join-Path $releaseDirectory "Musica-Nativa-Setup-v$Version.exe"
Copy-Item -LiteralPath $appExecutable -Destination $appAsset -Force
Copy-Item -LiteralPath $installer.FullName -Destination $installerAsset -Force

$assetNames = @($releaseExecutableName, "Musica-Nativa-Setup-v$Version.exe")
$checksumLines = foreach ($assetName in $assetNames) {
    $path = Join-Path $releaseDirectory $assetName
    $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $assetName"
}
Set-Content -LiteralPath (Join-Path $releaseDirectory 'SHA256SUMS') -Value $checksumLines -Encoding ASCII

Write-Output "Repositorio de releases: https://github.com/$releaseRepository/releases"
Get-ChildItem -LiteralPath $releaseDirectory -File | Select-Object FullName, Length
