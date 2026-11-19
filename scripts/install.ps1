$ErrorActionPreference = 'Stop'

$packageRoot = $PSScriptRoot
$appSource = Join-Path $packageRoot 'musica-nativa.exe'
$webViewBootstrapper = Join-Path $packageRoot 'MicrosoftEdgeWebview2Setup.exe'
$installRoot = Join-Path $env:LOCALAPPDATA 'Programs\Musica Nativa'
$startMenuFolder = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Musica Nativa'
$runtimeClient = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
$runtimePaths = @(
    "HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\$runtimeClient",
    "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\$runtimeClient",
    "HKCU:\SOFTWARE\Microsoft\EdgeUpdate\Clients\$runtimeClient"
)

if (-not (Test-Path -LiteralPath $appSource -PathType Leaf)) {
    throw 'No se encontró el ejecutable de Música Nativa dentro del instalador.'
}

$webViewInstalled = $false
foreach ($registryPath in $runtimePaths) {
    if (Test-Path -LiteralPath $registryPath) {
        $version = (Get-ItemProperty -LiteralPath $registryPath -Name pv -ErrorAction SilentlyContinue).pv
        if ($version) {
            $webViewInstalled = $true
            break
        }
    }
}

if (-not $webViewInstalled -and (Test-Path -LiteralPath $webViewBootstrapper -PathType Leaf)) {
    $runtimeSetup = Start-Process -FilePath $webViewBootstrapper -ArgumentList '/silent', '/install' -PassThru -Wait
    if ($runtimeSetup.ExitCode -ne 0) {
        throw "No se pudo instalar WebView2 (código $($runtimeSetup.ExitCode))."
    }
}

New-Item -ItemType Directory -Force -Path $installRoot, $startMenuFolder | Out-Null
$appPath = Join-Path $installRoot 'musica-nativa.exe'
Copy-Item -LiteralPath $appSource -Destination $appPath -Force
Copy-Item -LiteralPath (Join-Path $packageRoot 'uninstall.ps1') -Destination (Join-Path $installRoot 'uninstall.ps1') -Force

$shell = New-Object -ComObject WScript.Shell
$appShortcut = $shell.CreateShortcut((Join-Path $startMenuFolder 'Música Nativa.lnk'))
$appShortcut.TargetPath = $appPath
$appShortcut.WorkingDirectory = $installRoot
$appShortcut.IconLocation = "$appPath,0"
$appShortcut.Save()

$uninstallScript = Join-Path $installRoot 'uninstall.ps1'
$uninstallShortcut = $shell.CreateShortcut((Join-Path $startMenuFolder 'Desinstalar Música Nativa.lnk'))
$uninstallShortcut.TargetPath = Join-Path $PSHOME 'powershell.exe'
$uninstallShortcut.Arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$uninstallScript`""
$uninstallShortcut.WorkingDirectory = $installRoot
$uninstallShortcut.Save()

$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\MusicaNativa'
New-Item -Path $uninstallKey -Force | Out-Null
Set-ItemProperty -Path $uninstallKey -Name DisplayName -Value 'Música Nativa'
Set-ItemProperty -Path $uninstallKey -Name DisplayVersion -Value '1.0.4'
Set-ItemProperty -Path $uninstallKey -Name Publisher -Value 'Música Nativa'
Set-ItemProperty -Path $uninstallKey -Name DisplayIcon -Value $appPath
Set-ItemProperty -Path $uninstallKey -Name UninstallString -Value "`"$(Join-Path $PSHOME 'powershell.exe')`" -NoProfile -ExecutionPolicy Bypass -File `"$uninstallScript`""
Set-ItemProperty -Path $uninstallKey -Name NoModify -Type DWord -Value 1
Set-ItemProperty -Path $uninstallKey -Name NoRepair -Type DWord -Value 1

try {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show('Música Nativa quedó instalada. Encontrarás el acceso directo en el menú Inicio.', 'Música Nativa') | Out-Null
} catch {
    Write-Output 'Música Nativa quedó instalada.'
}
