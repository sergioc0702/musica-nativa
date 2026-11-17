param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$goBin = Join-Path $env:LOCALAPPDATA 'Programs\go\bin'
$gccBin = Join-Path $env:LOCALAPPDATA 'Programs\WinLibs\mingw64\bin'
$wailsBin = Join-Path $env:USERPROFILE 'go\bin'
$env:PATH = "$goBin;$gccBin;$wailsBin;$env:PATH"
$wails = Join-Path $wailsBin 'wails3.exe'
$outputDirectory = Join-Path $projectRoot 'bin'
$appExecutable = Join-Path $outputDirectory 'musica-nativa.exe'
$iexpress = Join-Path $env:WINDIR 'System32\iexpress.exe'
$buildStamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$installerOutput = Join-Path $outputDirectory "Musica-Nativa-Setup-$buildStamp.exe"
$stagingRoot = Join-Path $env:TEMP "MusicaNativa-Setup-$([Guid]::NewGuid().ToString('N'))"

if (-not (Test-Path -LiteralPath $wails -PathType Leaf)) {
    throw "No se encontró Wails CLI: $wails"
}
if (-not (Test-Path -LiteralPath $iexpress -PathType Leaf)) {
    throw 'IExpress no está disponible en este Windows.'
}
if (-not $SkipBuild) {
    Push-Location $projectRoot
    try {
        & $wails task windows:build PRODUCTION=true ARCH=amd64
        if ($LASTEXITCODE -ne 0) {
            throw "La compilación de Música Nativa terminó con código $LASTEXITCODE."
        }
    } finally {
        Pop-Location
    }
}
if (-not (Test-Path -LiteralPath $appExecutable -PathType Leaf)) {
    throw "No se encontró el ejecutable compilado: $appExecutable"
}

New-Item -ItemType Directory -Path $stagingRoot | Out-Null
$packageSucceeded = $false
try {
    Copy-Item -LiteralPath $appExecutable -Destination (Join-Path $stagingRoot 'musica-nativa.exe')
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'install.ps1') -Destination (Join-Path $stagingRoot 'install.ps1')
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'uninstall.ps1') -Destination (Join-Path $stagingRoot 'uninstall.ps1')

    Push-Location $projectRoot
    try {
        & $wails generate webview2bootstrapper -dir $stagingRoot
        if ($LASTEXITCODE -ne 0) {
            throw "No se pudo preparar el instalador de WebView2 (código $LASTEXITCODE)."
        }
    } finally {
        Pop-Location
    }

    $sedPath = Join-Path $stagingRoot 'package.sed'
    $sed = @"
[Version]
Class=IEXPRESS
SEDVersion=3

[Options]
PackagePurpose=InstallApp
ShowInstallProgramWindow=0
HideExtractAnimation=1
UseLongFileName=1
InsideCompressed=1
CAB_FixedSize=0
CAB_ResvCodeSigning=0
RebootMode=N
InstallPrompt=
DisplayLicense=
FinishMessage=%FinishMessage%
TargetName=%TargetName%
FriendlyName=%FriendlyName%
AppLaunched=%AppLaunched%
PostInstallCmd=<None>
AdminQuietInstCmd=
UserQuietInstCmd=
SourceFiles=SourceFiles

[Strings]
FILE0="musica-nativa.exe"
FILE1="install.ps1"
FILE2="uninstall.ps1"
FILE3="MicrosoftEdgeWebview2Setup.exe"
FinishMessage=Musica Nativa se instalo.
TargetName=$installerOutput
FriendlyName=Musica Nativa
AppLaunched=cmd.exe /c powershell.exe -NoProfile -ExecutionPolicy Bypass -File install.ps1

[SourceFiles]
SourceFiles0=$stagingRoot\

[SourceFiles0]
%FILE0%=
%FILE1%=
%FILE2%=
%FILE3%=
"@
    Set-Content -LiteralPath $sedPath -Value $sed -Encoding ASCII

    $iexpressProcess = Start-Process -FilePath $iexpress `
        -ArgumentList '/N /Q /M package.sed' `
        -WorkingDirectory $stagingRoot `
        -Wait -PassThru
    if ($iexpressProcess.ExitCode -ne 0) {
        throw "IExpress terminó con código $($iexpressProcess.ExitCode)."
    }
    if (-not (Test-Path -LiteralPath $installerOutput -PathType Leaf)) {
        throw 'IExpress no creó el archivo del instalador.'
    }
    $packageSucceeded = $true
    Get-Item -LiteralPath $installerOutput | Select-Object FullName, Length
} finally {
    $resolvedStage = [IO.Path]::GetFullPath($stagingRoot).TrimEnd('\')
    $resolvedTemp = [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\')
    $stagePrefix = $resolvedTemp + '\MusicaNativa-Setup-'
    if ($resolvedStage.StartsWith($stagePrefix, [StringComparison]::OrdinalIgnoreCase) -and $packageSucceeded) {
        Remove-Item -LiteralPath $resolvedStage -Recurse -Force -ErrorAction SilentlyContinue
    } elseif ($resolvedStage.StartsWith($stagePrefix, [StringComparison]::OrdinalIgnoreCase)) {
        Write-Warning "Se conservaron los archivos temporales para diagnosticar el paquete: $resolvedStage"
    }
}
