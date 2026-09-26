#Requires -Version 7.0
<#
.SYNOPSIS
    Shared helpers of scripts/demo.ps1 and scripts/demo-two-node.ps1 (dot-sourced by them; not run on its own).

.DESCRIPTION
    A demo script hands its steps to Invoke-DemoRun: the checks, the dashboard build and its copy into client/dist,
    dev builds of the server (the "<VERSION>-dev" stamp of the Makefile's dev targets), the seed, the nodes and the
    ready lines. Invoke-DemoRun then waits for Ctrl+C or Ctrl+Break, stops the nodes and returns the exit code. Every
    HTTP call goes to 127.0.0.1, never through a proxy.

    Settings other than the committed dev defaults (another port, the clinic's peer address) reach a build through
    "go build -overlay": the overlay adds internal/config/local.env or cloud.env from a temporary file outside the
    checkout, so nothing is written into internal/config.

    Stopping (the Windows parts are in scripts/native-process.cs):
    - Each server runs in its own process group on the script's console, with its output in server-console.log in
      its data folder. One Ctrl+Break stops it cleanly; a second one during its shutdown can end it on the spot.
    - The script's console handler only records Ctrl+C, Ctrl+Break and a closing console; the script then stops the
      servers itself (Stop-DemoNodes).
    - Ctrl+C never reaches the servers (a new process group ignores it), so they get Ctrl+Break from the script.
      Ctrl+Break from the keyboard reaches them directly, one sent to the script's process group by a program
      doesn't; so after Ctrl+Break the script waits a moment and sends it only to the servers still listening on
      their port (a server that got the event closes its port first).
    - Every seed and server process is in a job that Windows closes when the script ends, so a killed script takes
      them with it (a hard stop).
    - A closing console (or log-off, shutdown) gives the servers the same event, and Windows ends the script soon
      after its handler returns; the handler waits up to 4 s for the servers to finish, while the script's own stop
      runs as after Ctrl+Break.
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
# A native command's exit code is checked by the script itself, never turned into an error.
$PSNativeCommandUseErrorActionPreference = $false

# Settings

# The repository folder with its case on disk: $PSScriptRoot keeps the case the caller typed, and go build matches
# the -overlay paths case-sensitively, so a differently cased path would drop the overlay without a word.
$DemoRepoRoot = (Get-Item -LiteralPath (Split-Path -Parent $PSScriptRoot)).FullName
$DemoPassword = 'demo123'
$DemoAdmin = 'jvance'
$DemoUsers = @(
    [pscustomobject]@{ User = 'jvance'; Role = 'admin' }
    [pscustomobject]@{ User = 'tmercer'; Role = 'staff' }
    [pscustomobject]@{ User = 'lhayes'; Role = 'nurse' }
    [pscustomobject]@{ User = 'mowens'; Role = 'nurse' }
)
# Each node's override file in internal/config; its committed dev defaults are <file>.defaults.
$DemoConfigFiles = @{ clinic = 'local.env'; cloud = 'cloud.env' }
$DemoToolTimeoutSec = 300      # a short tool call such as go list or node --version
$DemoSeedTimeoutMin = 10
$DemoHealthTimeoutSec = 90
$DemoBreakGraceMs = 2000       # after Ctrl+Break: time for the servers that got it to close their port
$DemoStopTimeoutSec = 30       # from Ctrl+Break to the server's exit
$DemoKillWaitSec = 15          # from a hard stop to the server's exit
$DemoHelperTimeoutMs = 30000   # the Ctrl+Break helper for a server on another console

# Native types

$DemoNativeSource = Join-Path $PSScriptRoot 'native-process.cs'
if ($IsWindows -and -not ('ZealScripts.ConsoleEvents' -as [type])) { Add-Type -Path $DemoNativeSource }

# Output: progress "demo: <step>: ...", warnings "demo: warning: ..." and errors "demo: ERROR: ..." (on stderr)

function Write-DemoLine([string]$Text) { Write-Host "demo: $Text" }

function Write-DemoWarning([string]$Text) { [Console]::Error.WriteLine("demo: warning: $Text") }

function Write-DemoError([string]$Text) { [Console]::Error.WriteLine("demo: ERROR: $Text") }

function Read-DemoLog([string]$Path) {
    <# The text of a log file, also one a running server still writes to ('' when there is none yet). #>
    try {
        $share = [System.IO.FileShare]::ReadWrite -bor [System.IO.FileShare]::Delete
        $fs = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, $share)
        try { return [System.IO.StreamReader]::new($fs).ReadToEnd() } finally { $fs.Dispose() }
    } catch [System.IO.FileNotFoundException] { return '' } catch [System.IO.DirectoryNotFoundException] { return '' }
}

function Get-DemoLogTail([string]$Path, [int]$Lines = 20) {
    $text = (Read-DemoLog $Path).TrimEnd()
    if (-not $text) { return '  (no log)' }
    $tail = ($text -split '\r?\n') | Select-Object -Last $Lines | ForEach-Object { "  $_" }
    return $tail -join [Environment]::NewLine
}

function Format-DemoNodeExit($Node, [string]$What) {
    <# "the <node> node <What> (exit <code>)" and the last lines of its console log. #>
    return "the $($Node.Name) node $What (exit $($Node.Child.ExitCode)); last lines of $($Node.Log):" +
        "`n$(Get-DemoLogTail $Node.Log)"
}

# Stop requests: Ctrl+C, Ctrl+Break or a closing console, recorded by the console handler

function Test-DemoStopRequested { return [ZealScripts.ConsoleEvents]::IsSet }

function Assert-DemoNoStopRequest {
    <# Ends the current step once a stop was requested; Invoke-DemoRun then stops the demo. #>
    if (Test-DemoStopRequested) { throw [System.OperationCanceledException]::new('stop requested') }
}

function Wait-DemoStopRequest([object[]]$Nodes) {
    <# Blocks until a stop is requested. A node that stops on its own before that is a failure. #>
    while (-not [ZealScripts.ConsoleEvents]::Received.WaitOne(500)) {
        $exited = @($Nodes | Where-Object { $_.Child.HasExited })
        if (-not $exited.Count) { continue }
        # A console event can reach a node a moment before the script notices it.
        if ([ZealScripts.ConsoleEvents]::Received.WaitOne(1000)) { return }
        throw (Format-DemoNodeExit $exited[0] 'stopped on its own')
    }
}

# Paths

function Resolve-DemoPath([string]$Path) {
    <# A path given on the command line, relative to the current folder. #>
    return [System.IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Path))
}

function Test-DemoPathUnder([string]$Path, [string]$Root) {
    $full = [System.IO.Path]::GetFullPath($Path).TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar
    $r = [System.IO.Path]::GetFullPath($Root).TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar
    return $full.StartsWith($r, [System.StringComparison]::OrdinalIgnoreCase)
}

function Resolve-DemoFrontend([string]$Frontend) {
    <# The dashboard checkout: -Frontend, or ../zeal-clinic-frontend next to this repository. #>
    $default = [System.IO.Path]::GetFullPath((Join-Path (Split-Path -Parent $DemoRepoRoot) 'zeal-clinic-frontend'))
    $path = if ($Frontend) { Resolve-DemoPath $Frontend } else { $default }
    if (-not (Test-Path -LiteralPath (Join-Path $path 'package.json') -PathType Leaf)) {
        throw ("no dashboard checkout at $path. By default the demo builds the dashboard from " +
            "../zeal-clinic-frontend next to this repository ($default); clone it there, or pass -Frontend <path> to " +
            'use another folder.')
    }
    return $path
}

function Resolve-DemoBinDir([string]$BinDir) {
    if ($BinDir) { return Resolve-DemoPath $BinDir }
    return Join-Path $DemoRepoRoot 'tmp'
}

function Get-DemoDefaultPort([ValidateSet('clinic', 'cloud')][string]$Node) {
    <# The PORT of the node's committed dev defaults (the only key read from that file). #>
    $file = Join-Path $DemoRepoRoot "internal/config/$($DemoConfigFiles[$Node]).defaults"
    foreach ($line in [System.IO.File]::ReadAllLines($file)) {
        if ($line -match '^\s*PORT\s*=\s*"?(\d+)"?\s*$') { return [int]$Matches[1] }
    }
    throw "$file has no PORT line: the demo takes the $Node node's default port from it."
}

function Get-DemoLanAddress {
    <# The machine's primary IPv4 address on the LAN (127.0.0.1 fallback when offline or undetected). #>
    $udp = [System.Net.Sockets.UdpClient]::new()
    try {
        $udp.Connect('8.8.8.8', 80)
        $addr = $udp.Client.LocalEndPoint.Address.IPAddressToString
        if ($addr -and $addr -ne '0.0.0.0' -and $addr -ne '127.0.0.1') { return $addr }
    } catch { } finally { $udp.Dispose() }
    try {
        $ips = @([System.Net.Dns]::GetHostAddresses([System.Net.Dns]::GetHostName()) |
            Where-Object { $_.AddressFamily -eq [System.Net.Sockets.AddressFamily]::InterNetwork -and
                -not [System.Net.IPAddress]::IsLoopback($_) })
        if ($ips.Count) { return $ips[0].IPAddressToString }
    } catch { }
    return '127.0.0.1'
}

# Checks

function Assert-DemoWindows {
    if (-not $IsWindows) {
        throw ('this script runs on Windows: the clinic node is a Windows program. On Linux, scripts/demo-cloud.sh ' +
            'starts the cloud node.')
    }
}

function Assert-DemoNoOverride {
    <# The demo runs on the committed dev defaults only, and every build embeds a local override file. #>
    $found = @(Get-ChildItem -LiteralPath (Join-Path $DemoRepoRoot 'internal/config') -File -Force | Where-Object {
            $_.Name -match '^(local|cloud)\.env' -and $_.Name -notmatch '^(local|cloud)\.env\.(defaults|example)$'
        } | ForEach-Object Name)
    if ($found.Count) {
        throw ("internal/config holds $($found -join ', '). Every build embeds such an override file, which may " +
            'hold real values, and the demo runs on the committed dev defaults only: move it out of the checkout, or ' +
            'run the demo from a clean clone.')
    }
}

function Test-DemoListening([int]$Port) {
    $listeners = [System.Net.NetworkInformation.IPGlobalProperties]::GetIPGlobalProperties().GetActiveTcpListeners()
    return @($listeners | Where-Object { $_.Port -eq $Port }).Count -gt 0
}

function Format-DemoProcess([int]$Id) {
    $p = Get-Process -Id $Id -ErrorAction SilentlyContinue
    if (-not $p) { return "pid $Id" }
    $path = try { $p.Path } catch { $null }
    return "$($p.ProcessName) (pid $Id$(if ($path) { ", $path" }))"
}

function Assert-DemoPortFree([int]$Port, [string]$Flag) {
    <# Refuses a port something listens on, naming the listener. $Flag names the script's parameter for the port. #>
    if (-not (Test-DemoListening $Port)) { return }
    $owners = @()
    try {
        $owners = @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction Stop |
                Select-Object -ExpandProperty OwningProcess -Unique | ForEach-Object { Format-DemoProcess $_ })
    } catch { }
    $by = if ($owners.Count) { " by $($owners -join ', ')" } else { '' }
    $hint = if ($Flag) { "pick another port with $Flag <n>" } else { "start the demo again once it's free" }
    throw "port $Port is in use$by. Stop it, or $hint."
}

function Assert-DemoNoClinicRunning {
    $m = $null
    if (-not [System.Threading.Mutex]::TryOpenExisting('ZealClinic-singleton-lock', [ref]$m)) { return }
    $m.Dispose()
    $running = @(Get-Process -Name 'ZealClinic*' -ErrorAction SilentlyContinue |
            ForEach-Object { Format-DemoProcess $_.Id })
    $which = if ($running.Count) { "; running now: $($running -join ', ')" } else { '' }
    throw ('another clinic server is running on this machine (the installed app or another dev run holds its ' +
        "single-instance lock$which); stop it first.")
}

function Assert-DemoPrerequisites([string]$Frontend, [System.Collections.Specialized.OrderedDictionary]$Ports) {
    <#
      Every check before the builds: no local override, the tools (returned), each port ($Ports maps the script's
      parameter to its value) usable, different and free, and no other clinic server running.
    #>
    Assert-DemoNoOverride
    $tools = Get-DemoTools $Frontend
    foreach ($flag in $Ports.Keys) {
        $port = $Ports[$flag]
        if ($port -lt 1024 -or $port -gt 65535) { throw "$flag $port isn't a usable port (1024-65535)." }
        $same = @($Ports.Keys | Where-Object { $_ -ne $flag -and $Ports[$_] -eq $port })
        if ($same.Count) { throw "$flag and $($same -join ', ') are both $port; each node needs a port of its own." }
        Assert-DemoPortFree $port $flag
    }
    Assert-DemoNoClinicRunning
    return $tools
}

# Tools

function Invoke-DemoCapture([string]$Exe, [string[]]$Arguments) {
    <# Runs a short command in the repository and returns its exit code and output (stdout and stderr). #>
    $psi = [System.Diagnostics.ProcessStartInfo]::new($Exe)
    foreach ($a in $Arguments) { $psi.ArgumentList.Add($a) }
    $psi.WorkingDirectory = $DemoRepoRoot
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    $out = $p.StandardOutput.ReadToEndAsync()
    $err = $p.StandardError.ReadToEndAsync()
    if (-not $p.WaitForExit($DemoToolTimeoutSec * 1000)) {
        try { $p.Kill($true) } catch { }
        throw "$(Split-Path -Leaf $Exe) $($Arguments -join ' ') didn't finish within $DemoToolTimeoutSec s"
    }
    $p.WaitForExit()
    return [pscustomobject]@{ ExitCode = $p.ExitCode; Output = ($out.Result + $err.Result).Trim() }
}

function Invoke-DemoTool([string]$Exe, [string[]]$Arguments, [string]$Cwd, [string]$What) {
    <# Runs a build tool with its output on the console. A stop request during the run wins over its exit code. #>
    Push-Location -LiteralPath $Cwd
    try {
        $global:LASTEXITCODE = 0
        & $Exe @Arguments | Out-Host
        $code = $LASTEXITCODE
    } finally {
        Pop-Location
    }
    Assert-DemoNoStopRequest
    if ($code -ne 0) { throw "$What failed (exit $code); see its output above." }
}

function ConvertTo-DemoVersion([string]$Text) {
    <# The first "major.minor[.patch]" in $Text ("v24.14.1", "go1.26.8", "1.26"; a missing patch counts as 0). #>
    if ($Text -notmatch '(\d+)\.(\d+)(?:\.(\d+))?') { return $null }
    $patch = if ($Matches[3]) { [int]$Matches[3] } else { 0 }
    return [version]::new([int]$Matches[1], [int]$Matches[2], $patch)
}

function Test-DemoVersionRange([version]$Version, [string]$Range) {
    <#
      A node-semver range as in package.json engines: alternatives joined by "||", each a space-separated list of
      comparators (^, ~, >=, >, <=, <, = or a bare version; partial versions and x wildcards allowed).
      Returns $true or $false, or $null for a form this check doesn't read.
    #>
    foreach ($alt in ($Range -split '\|\|')) {
        $alt = $alt.Trim()
        if ($alt -in '', '*', 'x', 'X') { return $true }
        $all = $true
        foreach ($c in ($alt -split '\s+')) {
            if ($c -notmatch '^(\^|~|>=|<=|>|<|=)?v?(\d+)(?:\.(\d+|[xX*]))?(?:\.(\d+|[xX*]))?$') { return $null }
            $op = "$($Matches[1])"
            $parts = [System.Collections.Generic.List[int]]::new()
            $parts.Add([int]$Matches[2])
            foreach ($g in @($Matches[3], $Matches[4])) {
                if ("$g" -notmatch '^\d+$') { break }
                $parts.Add([int]$g)
            }
            $n = $parts.Count
            $maj = $parts[0]
            $min = if ($n -ge 2) { $parts[1] } else { 0 }
            $pat = if ($n -ge 3) { $parts[2] } else { 0 }
            $low = [version]::new($maj, $min, $pat)
            $next = switch ($n) {
                1 { [version]::new($maj + 1, 0, 0) }
                2 { [version]::new($maj, $min + 1, 0) }
                default { [version]::new($maj, $min, $pat + 1) }
            }
            $ok = switch ($op) {
                '>=' { $Version -ge $low }
                '>' { $Version -ge $next }
                '<' { $Version -lt $low }
                '<=' { $Version -lt $next }
                '~' {
                    $top = if ($n -ge 2) { [version]::new($maj, $min + 1, 0) } else { [version]::new($maj + 1, 0, 0) }
                    ($Version -ge $low) -and ($Version -lt $top)
                }
                '^' {
                    $top = if ($maj -gt 0 -or $n -eq 1) { [version]::new($maj + 1, 0, 0) }
                    elseif ($min -gt 0 -or $n -eq 2) { [version]::new(0, $min + 1, 0) }
                    else { [version]::new(0, 0, $pat + 1) }
                    ($Version -ge $low) -and ($Version -lt $top)
                }
                default { ($Version -ge $low) -and ($Version -lt $next) }
            }
            if (-not $ok) { $all = $false; break }
        }
        if ($all) { return $true }
    }
    return $false
}

function Get-DemoGo {
    <# The go command, checked against go.mod's go line. Returns Path, Version and a note on the check. #>
    $goLine = Select-String -LiteralPath (Join-Path $DemoRepoRoot 'go.mod') -Pattern '^go\s+(\S+)' |
        Select-Object -First 1
    $wanted = if ($goLine) { $goLine.Matches[0].Groups[1].Value } else { 'the version in go.mod' }
    $hint = "Install Go $wanted or newer (https://go.dev/dl/). An older Go (1.21 or newer) works too while " +
        "GOTOOLCHAIN allows the automatic toolchain (auto, the default): it then fetches Go $wanted by itself, which " +
        'needs the network once.'
    $go = Get-Command go -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $go) { throw "go wasn't found on PATH. $hint" }
    # go list -m reads go.mod: it fails when the Go in use is older and can't switch to the version go.mod asks for.
    $r = Invoke-DemoCapture $go.Source @('list', '-m')
    if ($r.ExitCode -ne 0) {
        throw "'go list -m' failed in the backend checkout (exit $($r.ExitCode)): $($r.Output). $hint"
    }
    $r = Invoke-DemoCapture $go.Source @('env', 'GOVERSION')
    if ($r.ExitCode -ne 0 -or -not $r.Output) {
        throw "'go env GOVERSION' failed (exit $($r.ExitCode)): $($r.Output). $hint"
    }
    $version = $r.Output
    $note = "go.mod asks for $wanted or newer"
    if ($version -match '^devel\b') {
        # A development build names no release to compare.
        $note = "a development build, not compared with go $wanted"
    } else {
        # A Go older than 1.21 reads go.mod without switching toolchains, so the comparison catches it.
        $have = ConvertTo-DemoVersion $version
        $need = ConvertTo-DemoVersion $wanted
        if ($have -and $need -and $have -lt $need) {
            throw "Go $version is older than the go $wanted that go.mod asks for. $hint"
        }
    }
    return [pscustomobject]@{ Path = $go.Source; Version = $version; Note = $note }
}

function Get-DemoNodeTools([string]$Frontend) {
    <#
      node and npm, each checked against the dashboard's engines range for it (when package.json names one). Returns
      node and npm, each with Path, Version and Range. npm runs as npm.cmd: PowerShell would pick npm.ps1, which
      re-reads its own command line.
    #>
    $package = Get-Content -LiteralPath (Join-Path $Frontend 'package.json') -Raw | ConvertFrom-Json
    $engines = Get-DemoField $package 'engines'
    $found = [ordered]@{}
    foreach ($tool in @(@{ Name = 'node'; Command = 'node' }, @{ Name = 'npm'; Command = 'npm.cmd' })) {
        $name = $tool.Name
        $range = "$(Get-DemoField $engines $name)"
        $cmd = Get-Command $tool.Command -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $cmd) {
            $wanted = if ($range) { " ($name $range)" } else { '' }
            throw "$name wasn't found on PATH: the dashboard build needs Node.js with npm$wanted (https://nodejs.org)."
        }
        $r = Invoke-DemoCapture $cmd.Source @('--version')
        $version = ConvertTo-DemoVersion $r.Output
        if ($r.ExitCode -ne 0 -or -not $version) {
            throw "'$name --version' failed (exit $($r.ExitCode)): $($r.Output)"
        }
        if ($range) {
            $match = Test-DemoVersionRange $version $range
            if ($match -eq $false) {
                throw ("$name $version doesn't match the dashboard's engines ($name $range): install a matching " +
                    'Node.js (https://nodejs.org).')
            }
            if ($null -eq $match) {
                Write-DemoWarning ("couldn't read the dashboard's engines range for $name ('$range'); going on with " +
                    "$name $version")
            }
        }
        $found[$name] = [pscustomobject]@{ Path = $cmd.Source; Version = $version; Range = $range }
    }
    return [pscustomobject]$found
}

function Get-DemoTools([string]$Frontend) {
    <# Checks go, node and npm, prints them in one line and returns the paths of go and npm. #>
    $go = Get-DemoGo
    $js = Get-DemoNodeTools $Frontend
    $engines = if ($js.node.Range) { " (dashboard engines: node $($js.node.Range))" } else { '' }
    Write-DemoLine "tools: $($go.Version) ($($go.Note)), node $($js.node.Version)$engines, npm $($js.npm.Version)"
    return [pscustomobject]@{ Go = $go.Path; Npm = $js.npm.Path }
}

# Builds

function Build-DemoDashboard([string]$Frontend, [string]$Npm) {
    <# npm ci and npm run build in the dashboard checkout, then its dist/ into client/dist as make frontend does. #>
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    Write-DemoLine "dashboard: npm ci (in $Frontend)"
    Invoke-DemoTool $Npm @('ci', '--prefer-offline', '--no-audit', '--no-fund') $Frontend 'npm ci'
    Write-DemoLine 'dashboard: npm run build'
    Invoke-DemoTool $Npm @('run', 'build') $Frontend 'npm run build'
    $dist = Join-Path $Frontend 'dist'
    if (-not (Test-Path -LiteralPath (Join-Path $dist 'index.html') -PathType Leaf)) {
        throw "npm run build left no dist/index.html in $Frontend"
    }

    # client/dist keeps its tracked .gitkeep placeholder (written back when missing, as make frontend writes it on
    # Windows); the rest is the new build.
    $target = Join-Path $DemoRepoRoot 'client/dist'
    New-Item -ItemType Directory -Force -Path $target | Out-Null
    Get-ChildItem -LiteralPath $target -Force | Where-Object Name -ne '.gitkeep' | Remove-Item -Recurse -Force
    Get-ChildItem -LiteralPath $dist -Force | Copy-Item -Destination $target -Recurse -Force
    $keep = Join-Path $target '.gitkeep'
    if (-not (Test-Path -LiteralPath $keep)) {
        [System.IO.File]::WriteAllText($keep, "The dashboard build is copied here by make frontend.`r`n")
    }
    $count = @(Get-ChildItem -LiteralPath $target -Recurse -File -Force | Where-Object Name -ne '.gitkeep').Count
    Write-DemoLine ('dashboard: dist/ copied into client/dist ({0} files; {1:0} s)' -f $count, $sw.Elapsed.TotalSeconds)
}

function Get-DemoVersionStamp {
    <# "<VERSION>-dev", the stamp of the Makefile's dev builds. #>
    $v = ([System.IO.File]::ReadAllText((Join-Path $DemoRepoRoot 'VERSION'))).Trim()
    if ($v -notmatch '^[0-9A-Za-z][0-9A-Za-z.+-]*$') { throw "VERSION holds an unexpected value: '$v'" }
    return "$v-dev"
}

function New-DemoBuildOverlay([string]$ConfigFile, [string[]]$Lines) {
    <#
      A temporary folder outside the checkout (zeal-demo-<12 hex digits> in TEMP) holding the node's override file with
      $Lines and overlay.json, which adds that file to a build as internal/config/<ConfigFile>. Returns Folder (the
      caller deletes it after the build) and File (the overlay.json path).
    #>
    $name = 'zeal-demo-' + [guid]::NewGuid().ToString('N').Substring(0, 12)
    $folder = Join-Path ([System.IO.Path]::GetTempPath()) $name
    if (Test-DemoPathUnder $folder $DemoRepoRoot) {
        throw "the temporary folder $folder is inside the checkout; point TEMP elsewhere"
    }
    New-Item -ItemType Directory -Path $folder | Out-Null
    try {
        $utf8 = [System.Text.UTF8Encoding]::new($false)
        $envPath = Join-Path $folder $ConfigFile
        [System.IO.File]::WriteAllText($envPath, (($Lines -join "`n") + "`n"), $utf8)
        $overlay = Join-Path $folder 'overlay.json'
        $replace = @{ (Join-Path $DemoRepoRoot "internal\config\$ConfigFile") = $envPath }
        [System.IO.File]::WriteAllText($overlay, (@{ Replace = $replace } | ConvertTo-Json -Compress), $utf8)
    } catch {
        Remove-Item -LiteralPath $folder -Recurse -Force -ErrorAction SilentlyContinue
        throw
    }
    return [pscustomobject]@{ Folder = $folder; File = $overlay }
}

function Build-DemoServer {
    <#
      go build of ./cmd/server into <BinDir>\<Name>.exe with the dev stamp (-tags cloud for the cloud node). A port
      other than the node's default and a peer address reach the build through an overlay (New-DemoBuildOverlay).
      Returns the build: Node, Name, Exe, Port, Stamp, and the override file and keys the server must report.
    #>
    param(
        [Parameter(Mandatory)][string]$Go,
        [Parameter(Mandatory)][ValidateSet('clinic', 'cloud')][string]$Node,
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$BinDir,
        [Parameter(Mandatory)][int]$Port,
        [string]$PeerUrl
    )
    $stamp = Get-DemoVersionStamp
    $configFile = $DemoConfigFiles[$Node]
    $settings = [ordered]@{}
    if ($PeerUrl) { $settings['PEER_URL'] = $PeerUrl }
    if ($Port -ne (Get-DemoDefaultPort $Node)) { $settings['PORT'] = "$Port" }
    $pairs = @(foreach ($key in $settings.Keys) { "$key=$($settings[$key])" })

    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $exe = Join-Path $BinDir "$Name.exe"
    $goArgs = [System.Collections.Generic.List[string]]::new()
    $goArgs.Add('build')
    if ($Node -eq 'cloud') { $goArgs.AddRange([string[]]@('-tags', 'cloud')) }
    $goArgs.AddRange([string[]]@('-ldflags', "-X clinic-api/internal/buildmode.Version=$stamp", '-o', $exe))
    $overlay = $null
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $on = 'the committed dev defaults'
        if ($pairs.Count) {
            $overlay = New-DemoBuildOverlay $configFile $pairs
            $goArgs.Add("-overlay=$($overlay.File)")
            $on = "the dev defaults plus $($pairs -join ', ') (a build overlay adds internal/config/$configFile " +
                'from a temporary file; nothing is written into the checkout)'
        }
        $goArgs.Add('./cmd/server')
        Write-DemoLine "build: $Name.exe ($Node node, version $stamp) on $on"
        Invoke-DemoTool $Go $goArgs $DemoRepoRoot "go build of $Name.exe"
    } finally {
        if ($overlay) { Remove-Item -LiteralPath $overlay.Folder -Recurse -Force -ErrorAction SilentlyContinue }
    }
    # An override file that turned up during the build would be embedded in the binary.
    Assert-DemoNoOverride
    Write-DemoLine ('build: {0} ({1:0} s)' -f $exe, $sw.Elapsed.TotalSeconds)
    return [pscustomobject]@{
        Node = $Node; Name = $Name; Exe = $exe; Port = $Port; Stamp = $stamp
        ConfigFile = $configFile; ConfigKeys = [string[]]@($settings.Keys)
    }
}

# Nodes

function Initialize-DemoData([string]$Root, [switch]$Reset, [string[]]$Folders = @()) {
    <# The demo's data folder under tmp/demo (with -Reset deleted first) and its node folders in it. #>
    if (-not (Test-DemoPathUnder $Root (Join-Path $DemoRepoRoot 'tmp\demo'))) {
        throw "refusing a data folder outside tmp/demo: $Root"
    }
    if ($Reset -and (Test-Path -LiteralPath $Root)) {
        Write-DemoLine "reset: deleting $Root"
        Remove-Item -LiteralPath $Root -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $Root | Out-Null
    foreach ($folder in $Folders) { New-Item -ItemType Directory -Force -Path (Join-Path $Root $folder) | Out-Null }
}

function Invoke-DemoSeed($Build, [string]$Dir) {
    <#
      --dev --seed-only --demo in the node's data folder, its output in seed-console.log there: migrations, then the
      demo data. The demo data is versioned like the migrations, so on a kept database this changes nothing (and it
      finishes a seed that was cut short). A stop request waits for the seed to finish.
    #>
    $kept = Test-Path -LiteralPath (Join-Path $Dir 'tmp\clinic.db')
    $log = Join-Path $Dir 'seed-console.log'
    Write-DemoLine "seed: $($Build.Name).exe --dev --seed-only --demo (in $Dir)"
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $seedArgs = [string[]]@('--dev', '--seed-only', '--demo')
    $child = [ZealScripts.ConsoleProcess]::Start($Build.Exe, $seedArgs, $Dir, $null, $log, $true)
    try {
        $noted = $false
        while (-not $child.WaitForExit(500)) {
            if (-not $noted -and (Test-DemoStopRequested)) {
                Write-DemoLine 'stop requested: waiting for the seed step to finish, then stopping'
                $noted = $true
            }
            if ($sw.Elapsed.TotalMinutes -ge $DemoSeedTimeoutMin) {
                $child.Kill()
                throw ("the seed didn't finish within $DemoSeedTimeoutMin minutes; last lines of ${log}:" +
                    "`n$(Get-DemoLogTail $log)")
            }
        }
        $code = $child.ExitCode
    } finally {
        $child.Dispose()
    }
    Assert-DemoNoStopRequest
    if ($code -ne 0) { throw "the seed failed (exit $code); last lines of ${log}:`n$(Get-DemoLogTail $log)" }
    $state = if ($kept) { 'kept the existing demo data' } else { 'new demo data' }
    Write-DemoLine ('seed: {0} ({1:0.0} s)' -f $state, $sw.Elapsed.TotalSeconds)
}

function Start-DemoNode($Session, $Build, [string]$Dir, [switch]$Lan) {
    <#
      Starts the build with --dev in its data folder, its console output in a new server-console.log there, and adds
      the node to the session. The port is checked once more first: something may have taken it since the checks.
      With -Lan the server also gets --lan to bind every network interface instead of 127.0.0.1 only. The node's Url
      is the address to open it at: 127.0.0.1, or with -Lan this machine's LAN address.
    #>
    Assert-DemoPortFree $Build.Port
    $log = Join-Path $Dir 'server-console.log'
    $serverArgs = [System.Collections.Generic.List[string]]::new()
    $serverArgs.Add('--dev')
    if ($Lan) { $serverArgs.Add('--lan') }
    $hostName = if ($Lan) { Get-DemoLanAddress } else { '127.0.0.1' }
    Write-DemoLine ("start: $($Build.Node) node, $($Build.Name).exe $($serverArgs -join ' ') on port $($Build.Port) " +
        "(in $Dir)")
    $child = [ZealScripts.ConsoleProcess]::Start($Build.Exe, $serverArgs.ToArray(), $Dir, $null, $log, $true)
    $node = [pscustomobject]@{
        Name = $Build.Node; Port = $Build.Port; Stamp = $Build.Stamp; Child = $child; Log = $log; Healthy = $false
        ConfigFile = $Build.ConfigFile; ConfigKeys = $Build.ConfigKeys; ConfigChecked = $Build.ConfigKeys.Count -eq 0
        Url = "http://${hostName}:$($Build.Port)"
    }
    $Session.Nodes.Add($node)
    if (-not $child.EndsWithCaller) {
        Write-DemoWarning ("the $($node.Name) node (pid $($child.Pid)) isn't tied to this script: if the script is " +
            'killed, stop the node yourself')
    }
    return $node
}

function Assert-DemoNodeConfig($Node, [switch]$Final) {
    <#
      A node built with overlay settings must log "[config] <file> sets <keys>" with each of them: go build drops an
      overlay without a word when its paths don't match the build's (see $DemoRepoRoot). Before -Final only a complete
      line (one that ends with a line break) is judged, never one the server is still writing; with -Final a missing
      line fails too.
    #>
    if ($Node.ConfigChecked) { return }
    $keys = $Node.ConfigKeys -join ', '
    $found = [regex]::Match((Read-DemoLog $Node.Log), '\[config\][^\r\n]*(\r?\n)?')
    $name = $Node.Name
    if (-not ($found.Success -and ($Final -or $found.Groups[1].Success))) {
        if ($Final) { throw "the $name node's log ($($Node.Log)) has no [config] line to confirm its settings ($keys)" }
        return
    }
    $line = $found.Value.Trim()
    $file = [regex]::Escape($Node.ConfigFile)
    switch -Regex ($line) {
        "^\[config\] $file sets (.+)$" {
            $set = @($Matches[1].Trim() -split ',\s*')
            $missing = @($Node.ConfigKeys | Where-Object { $set -notcontains $_ })
            if ($missing.Count) {
                throw "the $name node lacks some of its build settings ($($missing -join ', ')); its log says: $line"
            }
        }
        "^\[config\] no ${file}:" {
            throw ("the $name node runs without the settings its build was given ($keys; its log says: $line). " +
                "The build overlay wasn't applied; go build matches its paths case-sensitively, so start the script " +
                "through the repository's path as it is on disk ($DemoRepoRoot).")
        }
        default {
            throw "the $name node couldn't read its config to confirm its settings ($keys); its log says: $line"
        }
    }
    $Node.ConfigChecked = $true
}

function Wait-DemoHealth($Node) {
    <# Waits until the node answers /health as this build, checking its config line on the way. #>
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $DemoHealthTimeoutSec) {
        Assert-DemoNoStopRequest
        if ($Node.Child.HasExited) { throw (Format-DemoNodeExit $Node 'exited while starting') }
        Assert-DemoNodeConfig $Node
        $health = $null
        try { $health = (Invoke-DemoApi -Port $Node.Port -Method GET -Path '/health' -TimeoutSec 3).Json } catch { }
        if ((Get-DemoField $health 'status') -eq 'ok') {
            $version = "$(Get-DemoField $health 'version')"
            if ($version -ne $Node.Stamp) {
                throw "port $($Node.Port) answers /health as version '$version', not this build ($($Node.Stamp))"
            }
            Assert-DemoNodeConfig $Node -Final
            $Node.Healthy = $true
            Write-DemoLine ('start: {0} node healthy on http://127.0.0.1:{1} ({2:0.0} s)' -f $Node.Name, $Node.Port,
                $sw.Elapsed.TotalSeconds)
            return
        }
        Start-Sleep -Milliseconds 300
    }
    throw "the $($Node.Name) node didn't answer /health within $DemoHealthTimeoutSec s; last lines of $($Node.Log):" +
        "`n$(Get-DemoLogTail $Node.Log)"
}

# Stopping (see "Stopping" at the top)

function Send-DemoCtrlBreak($Node) {
    <# Ctrl+Break to the node's process group: directly on the script's console, otherwise through the helper. #>
    $id = $Node.Child.Pid
    $native = [ZealScripts.ConsoleProcess]
    $sent = $native::SharesConsoleWith($id) -and $native::SendCtrlBreak($id)
    if (-not $sent) { $sent = $native::SendCtrlBreakThroughHelper($id, $DemoNativeSource, $DemoHelperTimeoutMs) }
    if (-not $sent) { Write-DemoWarning "couldn't send Ctrl+Break to the $($Node.Name) node (pid $id)" }
}

function Wait-DemoNodeStop($Node, [switch]$Report) {
    <#
      Waits for the node to exit after Ctrl+Break and stops it hard when it doesn't. A clean stop is exit code 0 with
      the server's shutdown line in its log; returns whether the stop was clean. With -Report the result is printed:
      the stop line, or an error with the node's log tail.
    #>
    $hard = -not $Node.Child.WaitForExit($DemoStopTimeoutSec * 1000)
    if ($hard) {
        Write-DemoWarning ("the $($Node.Name) node didn't stop within $DemoStopTimeoutSec s of Ctrl+Break; " +
            'stopping it hard (not a clean shutdown)')
        $Node.Child.Kill()
        $null = $Node.Child.WaitForExit($DemoKillWaitSec * 1000)
    }
    $clean = -not $hard -and $Node.Child.ExitCode -eq 0 -and (Read-DemoLog $Node.Log).Contains('Server shutting down')
    if ($Report) {
        if ($clean) { Write-DemoLine "stop: $($Node.Name) node stopped cleanly (exit 0; its log shows the shutdown)" }
        else { Write-DemoError (Format-DemoNodeExit $Node "didn't stop cleanly") }
    }
    return $clean
}

function Stop-DemoNodes([object[]]$Nodes, [switch]$Report) {
    <#
      Stops the nodes: one Ctrl+Break to each node still running, except to the healthy ones that got the event from
      Ctrl+Break or a closing console themselves (their port is closed), then waits for every node, also one that has
      exited already. Returns $true when every node stopped cleanly; -Report prints each node's result.
    #>
    if (-not $Nodes.Count) { return $true }
    $eventReachedNodes = (Test-DemoStopRequested) -and
        [ZealScripts.ConsoleEvents]::FirstType -ne [ZealScripts.ConsoleEvents]::CtrlC
    $running = @($Nodes | Where-Object { -not $_.Child.HasExited })
    if ($eventReachedNodes) {
        # A node that got the event closes its port first: give them a moment to do so.
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        while ($sw.ElapsedMilliseconds -lt $DemoBreakGraceMs -and
            @($running | Where-Object { $_.Healthy -and (Test-DemoListening $_.Port) }).Count) {
            Start-Sleep -Milliseconds 50
        }
    }
    foreach ($node in $running) {
        $gotEvent = $eventReachedNodes -and $node.Healthy -and -not (Test-DemoListening $node.Port)
        if (-not $node.Child.HasExited -and -not $gotEvent) { Send-DemoCtrlBreak $node }
    }
    $allClean = $true
    foreach ($node in $Nodes) {
        $allClean = (Wait-DemoNodeStop $node -Report:$Report) -and $allClean
        $node.Child.Dispose()
    }
    return $allClean
}

# HTTP (127.0.0.1 only, never through a proxy)

function Invoke-DemoApi {
    <# One HTTP call; returns Status, Json (when the answer parses) and Text. #>
    param(
        [Parameter(Mandatory)][int]$Port,
        [Parameter(Mandatory)][string]$Method,
        [Parameter(Mandatory)][string]$Path,
        $Body = $null,
        [string]$Token = '',
        [int]$TimeoutSec = 30
    )
    $headers = @{}
    if ($Token) { $headers['Authorization'] = "Bearer $Token" }
    $request = @{
        Uri = "http://127.0.0.1:$Port$Path"; Method = $Method; Headers = $headers; TimeoutSec = $TimeoutSec
        NoProxy = $true; SkipHttpErrorCheck = $true
    }
    if ($null -ne $Body) {
        $request['Body'] = $Body | ConvertTo-Json -Compress
        $request['ContentType'] = 'application/json'
    }
    $r = Invoke-WebRequest @request
    $text = "$($r.Content)"
    $json = $null
    try { $json = $text | ConvertFrom-Json } catch { }
    return [pscustomobject]@{ Status = [int]$r.StatusCode; Json = $json; Text = $text }
}

function Invoke-DemoLongPost([int]$Port, [string]$Path, [string]$Token, [int]$TimeoutSec) {
    <#
      A POST that may run for minutes: it ends at a stop request or after $TimeoutSec, and closing the connection
      cancels the work on the server. Same result as Invoke-DemoApi.
    #>
    $handler = [System.Net.Http.HttpClientHandler]::new()
    $handler.UseProxy = $false
    $client = [System.Net.Http.HttpClient]::new($handler)
    $client.Timeout = [System.Threading.Timeout]::InfiniteTimeSpan
    $cts = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds($TimeoutSec))
    $url = "http://127.0.0.1:$Port$Path"
    $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Post, $url)
    $response = $null
    try {
        if ($Token) {
            $request.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $Token)
        }
        $task = $client.SendAsync($request, $cts.Token)
        try {
            while (-not $task.Wait(250)) { if (Test-DemoStopRequested) { $cts.Cancel() } }
        } catch {
            Assert-DemoNoStopRequest
            if ($cts.IsCancellationRequested) { throw "POST $Path on port $Port didn't finish within $TimeoutSec s" }
            $inner = $_.Exception
            while ($inner.InnerException) { $inner = $inner.InnerException }
            throw "POST $Path on port $Port failed: $($inner.Message)"
        }
        Assert-DemoNoStopRequest
        $response = $task.Result
        $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        $json = $null
        try { $json = $text | ConvertFrom-Json } catch { }
        return [pscustomobject]@{ Status = [int]$response.StatusCode; Json = $json; Text = $text }
    } finally {
        if ($response) { $response.Dispose() }
        $request.Dispose()
        $cts.Dispose()
        $client.Dispose()
    }
}

function Get-DemoField($Object, [string[]]$Names) {
    <# Walks JSON properties by name (any case); $null when one is missing. #>
    $cur = $Object
    foreach ($name in $Names) {
        if ($null -eq $cur) { return $null }
        $prop = $cur.PSObject.Properties[$name]
        if (-not $prop) { return $null }
        $cur = $prop.Value
    }
    return $cur
}

function Get-DemoShortText([string]$Text) {
    $t = ($Text -replace '\s+', ' ').Trim()
    if ($t.Length -gt 300) { $t = $t.Substring(0, 300) + '...' }
    return $t
}

function Get-DemoToken($Node, [string]$User) {
    <# Signs in on the node as a demo user and returns the token. #>
    $body = @{ username = $User; password = $DemoPassword }
    $r = Invoke-DemoApi -Port $Node.Port -Method POST -Path '/api/auth/login' -Body $body
    $token = Get-DemoField $r.Json 'Data', 'token'
    if ($r.Status -ne 200 -or -not $token) {
        throw "sign-in as $User on the $($Node.Name) node answered $($r.Status): $(Get-DemoShortText $r.Text)"
    }
    return "$token"
}

# Run

function Open-DemoBrowser([string]$Url) {
    try { Start-Process $Url | Out-Null } catch {
        Write-DemoWarning "couldn't open a browser ($($_.Exception.Message)); open $Url yourself"
    }
}

function Write-DemoReady {
    <#
      The ready block: the ready line (exactly $Line, once; the run counts as ready from here), the sign-ins, $Notes,
      the data folder and the nodes' console logs, how to stop; then the browser unless -NoBrowser.
    #>
    param(
        [Parameter(Mandatory)]$Session,
        [Parameter(Mandatory)][string]$Line,
        [Parameter(Mandatory)][string]$DataDir,
        [Parameter(Mandatory)][string]$BrowserUrl,
        [string]$SignInWhere = '',
        [string[]]$Notes = @(),
        [switch]$NoBrowser
    )
    Write-Host ''
    Write-Host $Line
    $Session.Ready = $true
    Write-Host "Sign in$SignInWhere with the password $DemoPassword as:"
    foreach ($u in $DemoUsers) { Write-Host ('  {0,-9} {1}' -f $u.User, $u.Role) }
    foreach ($note in $Notes) { Write-Host $note }
    Write-Host "Data: $DataDir (kept between runs; -Reset starts over)"
    Write-Host "Logs: $(($Session.Nodes | Sort-Object Name | ForEach-Object Log) -join ', ')"
    Write-Host 'Press Ctrl+C to stop the demo.'
    Write-Host ''
    if (-not $NoBrowser) { Open-DemoBrowser $BrowserUrl }
}

function Invoke-DemoRun([scriptblock]$Steps) {
    <#
      Runs one demo and returns its exit code. $Steps gets the session (Nodes, Ready), runs the steps and ends with
      Write-DemoReady; the run then waits for a stop request and stops every node, reporting each node's stop when the
      stop was requested. After the ready line a clean stop prints "Demo stopped." and returns 0; a stop before it
      prints "Stopped before the demo was ready." and returns 1, and so does any failure after its error: a node that
      stops on its own, or one that doesn't stop cleanly.
    #>
    $session = [pscustomobject]@{ Nodes = [System.Collections.Generic.List[object]]::new(); Ready = $false }
    $eventsOn = $false
    $stopRequested = $false
    $clean = $false
    try {
        Assert-DemoWindows
        $eventsOn = [ZealScripts.ConsoleEvents]::Enable()
        if (-not $eventsOn) {
            Write-DemoWarning "couldn't install the console handler: Ctrl+C may end the script before the servers stop"
        }
        $null = & $Steps $session
        Wait-DemoStopRequest $session.Nodes
        $stopRequested = $true
    } catch {
        if ($eventsOn -and -not $session.Ready -and (Test-DemoStopRequested)) { $stopRequested = $true }
        else { Write-DemoError $_.Exception.Message }
    } finally {
        try {
            if ($stopRequested) { Write-DemoLine 'stopping' }
            $clean = Stop-DemoNodes $session.Nodes -Report:$stopRequested
        } catch {
            Write-DemoError "stopping the demo failed: $($_.Exception.Message)"
        } finally {
            if ($eventsOn) { [ZealScripts.ConsoleEvents]::Disable() }
        }
    }
    if (-not $stopRequested) { return 1 }
    if (-not $session.Ready) { Write-Host 'Stopped before the demo was ready.'; return 1 }
    if (-not $clean) { return 1 }
    Write-Host 'Demo stopped.'
    return 0
}
