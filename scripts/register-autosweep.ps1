<#
.SYNOPSIS
    Registers a scheduled auto-sweep task for low-disk Windows hosts.

.DESCRIPTION
    Creates a Windows Scheduled Task that runs mw-ww-cli --clean-safe --apply --json
    when disk free space falls below a threshold. Sends optional ntfy.sh alert
    only when drive enters warn/critical status.

.PARAMETER ThresholdFreeGB
    Free space threshold in GB to trigger sweep (default: 15).

.PARAMETER NtfyUrl
    Optional ntfy.sh topic URL for webhook notifications.

.PARAMETER Schedule
    Cron-like schedule for the task (default: "0 */4 * * *" - every 4 hours).

.PARAMETER MwWwCliPath
    Path to mw-ww-cli executable (default: auto-detect in PATH or current dir).

.PARAMETER TaskName
    Name of the scheduled task (default: "MwWwAutoSweep").
#>
param(
    [Parameter()]
    [int]$ThresholdFreeGB = 15,

    [Parameter()]
    [string]$NtfyUrl = "",

    [Parameter()]
    [string]$Schedule = "0 */4 * * *",

    [Parameter()]
    [string]$MwWwCliPath = "",

    [Parameter()]
    [string]$TaskName = "MwWwAutoSweep"
)

$ErrorActionPreference = "Stop"

function Resolve-CliPath {
    param([string]$Hint)
    if ($Hint -and (Test-Path $Hint)) { return $Hint }
    $candidates = @(
        "mw-ww-cli.exe",
        "mw-ww-cli",
        (Join-Path $PSScriptRoot "..\mw-ww-cli.exe"),
        (Join-Path $PSScriptRoot "..\..\mw-ww.exe"),
        (Join-Path (Get-Location) "mw-ww-cli.exe")
    )
    foreach ($c in $candidates) {
        $resolved = (Get-Command $c -ErrorAction SilentlyContinue).Source
        if ($resolved) { return $resolved }
    }
    throw "mw-ww-cli not found. Provide -MwWwCliPath or ensure it is in PATH."
}

function Get-DiskStatus {
    param([int]$ThresholdGB)
    $thresholdBytes = [int64]($ThresholdGB * 1GB)
    Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | ForEach-Object {
        $freeGB = [math]::Round($_.FreeSpace / 1GB, 2)
        $totalGB = [math]::Round($_.Size / 1GB, 2)
        $status = "ok"
        if ($_.FreeSpace -le $thresholdBytes) { $status = "critical" }
        elseif ($_.FreeSpace -le ($thresholdBytes * 2)) { $status = "warn" }
        [PSCustomObject]@{
            Drive       = $_.DeviceID
            FreeGB      = $freeGB
            TotalGB     = $totalGB
            FreeBytes   = $_.FreeSpace
            Status      = $status
        }
    }
}

function Send-NtfyAlert {
    param([string]$Url, [string]$Message, [string]$Priority = "default", [string[]]$Tags = @("broom"))
    if (-not $Url) { return }
    try {
        $body = $Message
        $headers = @{
            "Title"   = "mw-ww Auto-Sweep Alert"
            "Priority" = $Priority
            "Tags"    = ($Tags -join ",")
        }
        Invoke-RestMethod -Method Post -Uri $Url -Body $body -Headers $headers -TimeoutSec 10
    } catch {
        Write-Warning "ntfy notification failed: $_"
    }
}

function Register-Task {
    param(
        [string]$CliPath,
        [int]$ThresholdGB,
        [string]$NtfyUrl,
        [string]$Schedule,
        [string]$TaskName
    )

    $action = New-ScheduledTaskAction -Execute $CliPath -Argument @"
--clean-safe --apply --json --threshold-gb $ThresholdGB $(if ($NtfyUrl) { "--ntfy-url `"$NtfyUrl`"" })
"@

    $triggerParts = $Schedule -split " "
    if ($triggerParts.Count -eq 5) {
        $minute, $hour, $dom, $month, $dow = $triggerParts
        if ($minute -match "^\d+$" -and $hour -match "^\d+$") {
            $trigger = New-ScheduledTaskTrigger -Daily -At ([DateTime]::Today.AddHours([int]$hour).AddMinutes([int]$minute))
        } else {
            $trigger = New-ScheduledTaskTrigger -Daily -At "03:00"
        }
    } else {
        $trigger = New-ScheduledTaskTrigger -Daily -At "03:00"
    }

    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RunOnlyIfNetworkAvailable
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

    Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force
    Write-Host "Registered scheduled task: $TaskName"
}

$cliPath = Resolve-CliPath $MwWwCliPath
Write-Host "Using mw-ww-cli at: $cliPath"

$disks = Get-DiskStatus -ThresholdGB $ThresholdFreeGB
$criticalDrives = $disks | Where-Object { $_.Status -eq "critical" }
$warnDrives = $disks | Where-Object { $_.Status -eq "warn" }

if ($criticalDrives.Count -gt 0 -or $warnDrives.Count -gt 0) {
    $msg = "mw-ww auto-sweep triggered: "
    if ($criticalDrives.Count -gt 0) { $msg += "$($criticalDrives.Count) critical drive(s); " }
    if ($warnDrives.Count -gt 0) { $msg += "$($warnDrives.Count) warn drive(s)" }
    Send-NtfyAlert -Url $NtfyUrl -Message $msg -Priority "high" -Tags @("broom", "warning")
}

Register-Task -CliPath $cliPath -ThresholdGB $ThresholdFreeGB -NtfyUrl $NtfyUrl -Schedule $Schedule -TaskName $TaskName

Write-Host "Auto-sweep registration complete. Threshold: ${ThresholdFreeGB}GB"