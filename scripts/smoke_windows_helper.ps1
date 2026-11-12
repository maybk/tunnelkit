[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$InstallerPath
)

$ErrorActionPreference = "Stop"

$resolvedInstaller = (Resolve-Path -LiteralPath $InstallerPath).Path
$installDir = Join-Path $env:SystemDrive ("tunnelkit-smoke-" + [System.Guid]::NewGuid().ToString("N").Substring(0, 8))
$protectedBinDir = Join-Path $env:ProgramFiles "TunnelKit Helper"
$protectedStateDir = Join-Path $env:ProgramData "TunnelKit Helper"
$expectedSddlFragment = "(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)(A;;CCLCRPRC;;;IU)"

try {
    Write-Host "==> Running silent NSIS installer: $resolvedInstaller -> $installDir"
    $installProc = Start-Process -FilePath $resolvedInstaller -ArgumentList @("/S", "/D=$installDir") -Wait -PassThru
    if ($installProc.ExitCode -ne 0) {
        throw "NSIS silent install failed with exit code $($installProc.ExitCode)"
    }

    foreach ($file in @("tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll", "uninstall.exe")) {
        $fullPath = Join-Path $installDir $file
        if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
            throw "Expected installed file missing: $fullPath"
        }
    }

    foreach ($protectedFile in @("tunnelkit-helper.exe", "wintun.dll")) {
        $fullPath = Join-Path $protectedBinDir $protectedFile
        if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
            throw "Expected protected helper component missing: $fullPath"
        }
    }
    if (-not (Test-Path -LiteralPath $protectedStateDir -PathType Container)) {
        throw "Expected protected state directory missing: $protectedStateDir"
    }

    $svc = Get-Service -Name "TunnelKitHelper" -ErrorAction Stop
    if ($svc.StartType -ne [System.ServiceProcess.ServiceStartMode]::Manual) {
        throw "Expected TunnelKitHelper StartType=Manual, got $($svc.StartType)"
    }
    if ($svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Stopped) {
        throw "Expected TunnelKitHelper Status=Stopped after install, got $($svc.Status)"
    }

    $sdOut = (& sc.exe sdshow TunnelKitHelper | Out-String).Trim()
    Write-Host "==> TunnelKitHelper SDDL: $sdOut"
    if (-not $sdOut.Contains($expectedSddlFragment)) {
        throw "TunnelKitHelper SDDL did not match expected canonical rights.`nActual: $sdOut`nExpected fragment: $expectedSddlFragment"
    }

    $env:TUNNELKIT_SMOKE_HELPER_SOURCE = Join-Path $installDir "tunnelkit-helper.exe"
    Write-Host "==> Verifying privilege.Current and Named Pipe IPC session via Windows SCM"
    & go test -v -count=1 -run "^TestSmokeInstalledWindowsHelperIsCurrent$" ./internal/privilege
    if ($LASTEXITCODE -ne 0) {
        throw "TestSmokeInstalledWindowsHelperIsCurrent failed with exit code $LASTEXITCODE"
    }

    & go test -v -count=1 -run "^TestSmokeLaunchInstalledWindowsHelper$" ./internal/helperclient
    if ($LASTEXITCODE -ne 0) {
        throw "TestSmokeLaunchInstalledWindowsHelper failed with exit code $LASTEXITCODE"
    }

    $uninstaller = Join-Path $installDir "uninstall.exe"
    Write-Host "==> Running silent NSIS uninstaller: $uninstaller"
    $uninstallProc = Start-Process -FilePath $uninstaller -ArgumentList @("/S", "_?=$installDir") -Wait -PassThru
    if ($uninstallProc.ExitCode -ne 0) {
        throw "NSIS silent uninstall failed with exit code $($uninstallProc.ExitCode)"
    }

    if ($null -ne (Get-Service -Name "TunnelKitHelper" -ErrorAction SilentlyContinue)) {
        throw "TunnelKitHelper service still exists after silent uninstall"
    }
    if (Test-Path -LiteralPath $protectedBinDir) {
        throw "Protected helper directory still exists after silent uninstall: $protectedBinDir"
    }
}
finally {
    Remove-Item Env:\TUNNELKIT_SMOKE_HELPER_SOURCE -ErrorAction SilentlyContinue
    if ($null -ne (Get-Service -Name "TunnelKitHelper" -ErrorAction SilentlyContinue)) {
        $fallbackHelper = Join-Path $installDir "tunnelkit-helper.exe"
        if (Test-Path -LiteralPath $fallbackHelper -PathType Leaf) {
            & $fallbackHelper uninstall-service 2>$null
        }
        else {
            & sc.exe stop TunnelKitHelper >$null 2>&1
            & sc.exe delete TunnelKitHelper >$null 2>&1
        }
    }
    Remove-Item -LiteralPath $installDir -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $protectedBinDir -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $protectedStateDir -Recurse -Force -ErrorAction SilentlyContinue
}
