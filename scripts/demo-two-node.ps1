#Requires -Version 7.0
<#
.SYNOPSIS
    Runs a clinic node and a cloud node side by side on this machine, both with the demo data, syncing (Windows).

.DESCRIPTION
    pwsh scripts/demo-two-node.ps1 [-Frontend <path>] [-ClinicPort <n>] [-CloudPort <n>] [-Reset] [-NoBrowser]
                                   [-BinDir <dir>]

    Runs from any folder (the repository is found from this script's location; relative paths given on the command
    line are relative to the current folder):
      1. the checks of scripts/demo.ps1, for both ports;
      2. builds the dashboard (npm ci, npm run build in -Frontend, default ../zeal-clinic-frontend next to this
         repository) and copies its dist/ into client/dist, as make frontend does;
      3. builds the clinic node with PEER_URL=http://127.0.0.1:<cloud port> and the cloud node (-tags cloud), both
         dev builds (version "<VERSION>-dev") on the committed dev defaults, which give both the same sync secret.
         The clinic's peer address (and a port other than the default) reaches each build through a build overlay
         from a temporary file outside the checkout; nothing is written into internal/config;
      4. seeds the clinic's demo data into tmp/demo/two-node/clinic/ (kept between runs; -Reset deletes
         tmp/demo/two-node and seeds again); the cloud node starts from its own folder tmp/demo/two-node/cloud/;
      5. starts both with --dev and waits for /health, then signs in on the clinic as the demo admin and runs the
         cloud restore, which copies the clinic's data to the cloud; incremental sync starts from there (on the demo
         data it can't start without it, so it runs on every start, also with kept data). The restore may take up to
         300 s; Ctrl+C cancels it;
      6. waits until the cloud accepts financial writes (sync is running), prints "Demo ready: clinic <url>, cloud
         <url>", the demo sign-ins, the data folder and the servers' logs, and opens the clinic in the browser unless
         -NoBrowser.
    Ctrl+C or Ctrl+Break (from the keyboard, or sent to the script's process group) stops both nodes cleanly; the
    script then prints "Demo stopped." and exits with 0. A stop before the ready line prints "Stopped before the demo
    was ready." and exits with 1, as does any failure. A killed script takes both nodes with it.

    -ClinicPort, -CloudPort  default: PORT of internal/config/local.env.defaults and cloud.env.defaults.
    -BinDir  where the binaries go (ZealClinicDemoClinic.exe, ZealClinicDemoCloud.exe). Windows Firewall may ask once
             per binary path; the demo only needs 127.0.0.1, so the question can be cancelled.
#>
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$Frontend,
    [int]$ClinicPort = 0,
    [int]$CloudPort = 0,
    [switch]$Reset,
    [switch]$NoBrowser,
    [string]$BinDir
)

. (Join-Path $PSScriptRoot 'demo-common.ps1')

$DemoRestoreTimeoutSec = 300
$DemoSyncTimeoutSec = 120

function Invoke-DemoCloudRestore($Clinic) {
    <#
      Signs in on the clinic as the demo admin and runs POST /api/cloud-restore: the clinic uploads a snapshot of its
      database, which replaces the cloud's synced tables. A stop request or the time limit ends the call, and the
      clinic then cancels the restore.
    #>
    $token = Get-DemoToken $Clinic $DemoAdmin
    Write-DemoLine "restore: POST /api/cloud-restore on the clinic as $DemoAdmin (its data replaces the cloud's)"
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $timeout = $DemoRestoreTimeoutSec
    $r = Invoke-DemoLongPost -Port $Clinic.Port -Path '/api/cloud-restore' -Token $token -TimeoutSec $timeout
    if ($r.Status -ne 200 -or (Get-DemoField $r.Json 'success') -ne $true) {
        $answer = "$($r.Status): $(Get-DemoShortText $r.Text)"
        throw "the cloud restore answered $answer (logs: $($Clinic.Log) and the cloud's)"
    }
    $tables = Get-DemoField $r.Json 'data', 'tables'
    $rows = Get-DemoField $r.Json 'data', 'rows'
    Write-DemoLine ('restore: done, {0} tables and {1} rows on the cloud ({2:0.0} s)' -f $tables, $rows,
        $sw.Elapsed.TotalSeconds)
}

function Wait-DemoSyncRunning($Cloud, [object[]]$Nodes) {
    <#
      Signs in on the cloud as the demo admin and waits until sync runs. The cloud refuses financial writes (503, code
      sync_not_ready) until the clinic has finished a full sync cycle with it; an empty payment probes that without
      writing anything: once sync runs, validation refuses it (400).
    #>
    $token = Get-DemoToken $Cloud $DemoAdmin
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $last = ''
    while ($sw.Elapsed.TotalSeconds -lt $DemoSyncTimeoutSec) {
        Assert-DemoNoStopRequest
        $exited = @($Nodes | Where-Object { $_.Child.HasExited })
        if ($exited.Count) { throw (Format-DemoNodeExit $exited[0] 'exited') }
        $r = Invoke-DemoApi -Port $Cloud.Port -Method POST -Path '/api/client-payments' -Body @{} -Token $token
        if ($r.Status -eq 400) {
            Write-DemoLine ('sync: running; the cloud accepts financial writes ({0:0.0} s after the restore)' -f
                $sw.Elapsed.TotalSeconds)
            return
        }
        $code = Get-DemoField $r.Json 'code'
        if ($r.Status -ne 503 -or $code -ne 'sync_not_ready') {
            throw "unexpected answer while waiting for sync: $($r.Status) $(Get-DemoShortText $r.Text)"
        }
        $last = "$($r.Status) $code"
        Start-Sleep -Milliseconds 500
    }
    throw ("sync didn't start within $DemoSyncTimeoutSec s (the cloud still answers $last); see " +
        "$(($Nodes | ForEach-Object Log) -join ' and ')")
}

$steps = {
    param($Session)
    $frontendDir = Resolve-DemoFrontend $Frontend
    $binDir = Resolve-DemoBinDir $BinDir
    $clinicPort = if ($ClinicPort) { $ClinicPort } else { Get-DemoDefaultPort clinic }
    $cloudPort = if ($CloudPort) { $CloudPort } else { Get-DemoDefaultPort cloud }
    $clinicUrl = "http://127.0.0.1:$clinicPort"
    $cloudUrl = "http://127.0.0.1:$cloudPort"
    $dataRoot = Join-Path $DemoRepoRoot 'tmp\demo\two-node'
    $clinicDir = Join-Path $dataRoot 'clinic'
    $cloudDir = Join-Path $dataRoot 'cloud'

    $ports = [ordered]@{ '-ClinicPort' = $clinicPort; '-CloudPort' = $cloudPort }
    $tools = Assert-DemoPrerequisites -Frontend $frontendDir -Ports $ports
    Build-DemoDashboard -Frontend $frontendDir -Npm $tools.Npm
    $builds = @{ Go = $tools.Go; BinDir = $binDir }
    $clinicBuild = Build-DemoServer @builds -Node clinic -Name ZealClinicDemoClinic -Port $clinicPort -PeerUrl $cloudUrl
    $cloudBuild = Build-DemoServer @builds -Node cloud -Name ZealClinicDemoCloud -Port $cloudPort
    Initialize-DemoData -Root $dataRoot -Reset:$Reset -Folders clinic, cloud
    Invoke-DemoSeed -Build $clinicBuild -Dir $clinicDir
    # The cloud needs no seed step: the server creates and migrates its database on start, and the restore replaces
    # its synced tables with the clinic's.
    $cloud = Start-DemoNode -Session $Session -Build $cloudBuild -Dir $cloudDir
    Wait-DemoHealth $cloud
    $clinic = Start-DemoNode -Session $Session -Build $clinicBuild -Dir $clinicDir
    Wait-DemoHealth $clinic
    Invoke-DemoCloudRestore $clinic
    Wait-DemoSyncRunning $cloud $Session.Nodes

    $ready = @{
        Session = $Session; Line = "Demo ready: clinic $clinicUrl, cloud $cloudUrl"; SignInWhere = ' on either node'
        Notes = @(
            'The clinic shows one "Data sync failed" notice from before the first cloud restore; it can be ignored.'
            'The nodes sync both ways: a payment taken on the cloud shows up on the clinic, and the other way round.'
        )
        DataDir = $dataRoot; BrowserUrl = "$clinicUrl/"; NoBrowser = $NoBrowser
    }
    Write-DemoReady @ready
}

exit (Invoke-DemoRun $steps)
