$ErrorActionPreference = 'Stop'

$expectedRoot = [IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'Programs\Musica Nativa')).TrimEnd('\')
$installRoot = [IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\')
if (-not [string]::Equals($installRoot, $expectedRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'La ruta de instalación no coincide con Música Nativa; se canceló la desinstalación.'
}

$installItem = Get-Item -LiteralPath $installRoot -Force
if (($installItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    throw 'La carpeta de instalación es un enlace; se canceló la desinstalación.'
}

$startMenuFolder = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Musica Nativa'
if (Test-Path -LiteralPath $startMenuFolder -PathType Container) {
    Get-ChildItem -LiteralPath $startMenuFolder -Force | Remove-Item -Force
    Remove-Item -LiteralPath $startMenuFolder -Force
}

$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\MusicaNativa'
if (Test-Path -LiteralPath $uninstallKey) {
    Remove-Item -LiteralPath $uninstallKey -Recurse -Force
}

Remove-Item -LiteralPath $installRoot -Recurse -Force
try {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show('Música Nativa se desinstaló. La biblioteca y las preferencias locales se conservaron.', 'Música Nativa') | Out-Null
} catch {
    Write-Output 'Música Nativa se desinstaló.'
}
