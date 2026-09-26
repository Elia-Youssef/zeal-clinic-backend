#Requires -Version 7.0
<#
.SYNOPSIS
    Runs the clinic server on this machine with demo data (Windows).

.DESCRIPTION
    pwsh scripts/demo.ps1 [-Frontend <path>] [-Port <n>] [-Lan] [-Reset] [-NoBrowser] [-BinDir <dir>]

    Runs from any folder (the repository is found from this script's location; relative paths given on the command
    line are relative to the current folder):
      1. checks: go, node and npm (node and npm matching the dashboard's engines), the port is free, no other clinic
         server runs, and internal/config holds no local override (the demo runs on the committed dev defaults);
      2. builds the dashboard (npm ci, npm run build in -Frontend, default ../zeal-clinic-frontend next to this
         repository) and copies its dist/ into client/dist, as make frontend does;
      3. builds a dev server (version "<VERSION>-dev") into -BinDir (default tmp/);
      4. seeds demo data into tmp/demo/clinic/ (kept between runs; -Reset deletes the folder and seeds again);
      5. starts the server with --dev (plus --lan with -Lan) in that folder, waits for /health, prints
         "Demo ready: <url>", the demo sign-ins, the data folder and the server's log, and opens the browser unless
         -NoBrowser.
    Ctrl+C or Ctrl+Break (from the keyboard, or sent to the script's process group) stops the server cleanly; the
    script then prints "Demo stopped." and exits with 0. A stop before the ready line prints "Stopped before the demo
    was ready." and exits with 1, as does any failure. A killed script takes the server with it.

    -Port    the server port (default: PORT of internal/config/local.env.defaults). Another port reaches the build
             through a build overlay; nothing is written into internal/config.
    -Lan     opens the LAN: binds all network interfaces so other devices can connect (default: 127.0.0.1 only).
    -BinDir  where the binary goes (ZealClinicDemo.exe). Windows Firewall may ask once per binary path; the demo only
             needs 127.0.0.1 without -Lan, so the question can be cancelled.
#>
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$Frontend,
    [int]$Port = 0,
    [switch]$Lan,
    [switch]$Reset,
    [switch]$NoBrowser,
    [string]$BinDir
)

. (Join-Path $PSScriptRoot 'demo-common.ps1')

$steps = {
    param($Session)
    $frontendDir = Resolve-DemoFrontend $Frontend
    $binDir = Resolve-DemoBinDir $BinDir
    $clinicPort = if ($Port) { $Port } else { Get-DemoDefaultPort clinic }
    $dataDir = Join-Path $DemoRepoRoot 'tmp\demo\clinic'

    $tools = Assert-DemoPrerequisites -Frontend $frontendDir -Ports ([ordered]@{ '-Port' = $clinicPort })
    Build-DemoDashboard -Frontend $frontendDir -Npm $tools.Npm
    $build = Build-DemoServer -Go $tools.Go -BinDir $binDir -Node clinic -Name ZealClinicDemo -Port $clinicPort
    Initialize-DemoData -Root $dataDir -Reset:$Reset
    Invoke-DemoSeed -Build $build -Dir $dataDir
    $clinic = Start-DemoNode -Session $Session -Build $build -Dir $dataDir -Lan:$Lan
    Wait-DemoHealth $clinic

    $url = $clinic.Url
    $ready = @{
        Session = $Session; Line = "Demo ready: $url"; DataDir = $dataDir; BrowserUrl = "$url/"; NoBrowser = $NoBrowser
    }
    Write-DemoReady @ready
}

exit (Invoke-DemoRun $steps)
