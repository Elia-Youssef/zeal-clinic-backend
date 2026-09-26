#!/bin/sh
# Runs the cloud node on demo data (Linux).
#
#   scripts/demo-cloud.sh [--frontend <path>] [--port <n>] [--lan] [--reset] [--no-browser]
#
#   --frontend <path>  the dashboard checkout (default: ../zeal-clinic-frontend next to this checkout)
#   --port <n>         the server port (default: PORT of internal/config/cloud.env.defaults)
#   --lan              open the LAN: listen on all network interfaces (default: 127.0.0.1 only). In WSL with NAT
#                      networking (its default) the printed address is the VM's own, which other devices can't
#                      reach: they need WSL's mirrored networking or a port proxy on Windows.
#   --reset            delete the demo data in tmp/demo/cloud/ and seed it again
#   --no-browser       don't open a browser
#
# Runs from any folder (the checkout is found from this script's location; a relative --frontend is relative to
# the current folder):
#   1. checks: no local config override in internal/config (the demo runs on the committed dev defaults), the
#      port is free, go (as new as go.mod asks, or allowed to fetch that toolchain), node and npm (node matching
#      the dashboard's engines), curl;
#   2. builds the dashboard (npm ci, npm run build) and copies its dist/ into client/dist, as make frontend does;
#   3. builds the cloud node (go build -tags cloud, version "<VERSION>-dev") into tmp/ZealClinicDemoCloud; another
#      port reaches the build through a go build overlay in a temporary folder outside the checkout;
#   4. runs the versioned demo seed in tmp/demo/cloud/ (the first run seeds; later runs keep the data and only
#      apply what is new; --reset deletes the folder first);
#   5. starts the server in that folder with --dev (plus --lan when given), waits until /health on 127.0.0.1
#      answers as this build, and prints "Demo ready: <url>", the demo sign-ins and where the data and logs are.
# A cloud node without its clinic keeps financial writes read-only; the clinic node runs on Windows only
# (scripts/demo.ps1, scripts/demo-two-node.ps1).
#
# Stopping:
#   - The server runs as a child in the script's process group, with SIGHUP ignored. The script traps SIGINT,
#     SIGTERM and SIGHUP (Ctrl+C, a kill, a closed terminal) and stops the server itself.
#   - After the ready line the server gets SIGINT and shuts down cleanly (SIGTERM after stop_grace_s, SIGKILL
#     after stop_kill_s more); the script prints "Demo stopped." and exits with 0.
#   - Before the ready line a started server gets SIGTERM; the script prints "Stopped before the demo was
#     ready." and exits with 128 + the signal number (129, 130 or 143).
#   - The server stopping on its own is a failure: exit 1 with the last lines of its log.
#   - Started as a background job of a non-interactive shell, the script inherits SIGINT ignored and can't trap
#     it: stop it with SIGTERM. A SIGINT then reaches only the server, which counts as it stopping on its own.
#
# Output: progress as "demo: <step>: ...", problems as "demo: warning: ..." and "demo: ERROR: ..." on stderr.
# The server's own output goes to server-console.log in its data folder, the seed's to seed-console.log.
set -eu

# Settings

bin_name=ZealClinicDemoCloud
defaults_file=internal/config/cloud.env.defaults
health_timeout_s=90 # for the server to answer /health after its start
stop_grace_s=30     # for a clean shutdown after SIGINT, before SIGTERM
stop_kill_s=10      # after SIGTERM, before SIGKILL
demo_password=demo123
demo_users='jvance:admin tmercer:staff lhayes:nurse mowens:nurse'

# State, read by the stop handlers

server_pid=''  # the running server
ready=0        # 1 once the ready line is out
stopping=0     # 1 once a stop is under way
overlay_dir='' # the build overlay's temporary folder while it exists

# Output (say survives a closed terminal, so a stop after SIGHUP runs to its end)

say() { printf '%s\n' "$*" 2>/dev/null || true; }
step() { say "demo: $*"; }
warn() { say "demo: warning: $*" >&2; }
die() {
	say "demo: ERROR: $*" >&2
	exit 1
}

# die_log <log> <message>: the error, then the last lines of the log.
die_log() {
	log=$1
	shift
	say "demo: ERROR: $*; last lines of $log:" >&2
	if [ -f "$log" ]; then tail -n 20 -- "$log" 2>/dev/null | sed 's/^/  /' >&2 || true; else say '  (no log)' >&2; fi
	exit 1
}

usage() {
	say 'Usage: scripts/demo-cloud.sh [--frontend <path>] [--port <n>] [--lan] [--reset] [--no-browser]'
	say '  --frontend <path>  the dashboard checkout (default: ../zeal-clinic-frontend next to this checkout)'
	say '  --port <n>         the server port (default: PORT of internal/config/cloud.env.defaults)'
	say '  --lan              open the LAN: listen on all network interfaces (default: 127.0.0.1 only). In WSL with NAT'
	say "                     networking (its default) the printed address is the VM's own, which other devices can't"
	say "                     reach: they need WSL's mirrored networking or a port proxy on Windows."
	say '  --reset            delete the demo data in tmp/demo/cloud/ and seed it again'
	say "  --no-browser       don't open a browser"
}

# get_lan_ip: the machine's primary IPv4 address on the LAN (127.0.0.1 fallback when offline or undetected).
get_lan_ip() {
	ip=$(ip route get 8.8.8.8 2>/dev/null | awk '{for(i=1;i<=NF;i++)if($i=="src"){print $(i+1);exit}}')
	if [ -z "$ip" ]; then
		ip=$(hostname -I 2>/dev/null | awk '{print $1}')
	fi
	if [ -z "$ip" ]; then
		ip='127.0.0.1'
	fi
	printf '%s' "$ip"
}

# Time: clock reads the seconds since the epoch (with a fraction where date supports %N); since <reading> <decimals>
# prints the seconds gone by since a reading; passed <reading> <seconds> tells whether that many have.

clock() {
	now=$(date +%s.%N 2>/dev/null) || now=''
	case $now in '' | *[!0-9.]*) date +%s ;; *) say "$now" ;; esac
}
since() { awk -v a="$1" -v b="$(clock)" -v f="%.${2}f" 'BEGIN { printf f, b - a }'; }
passed() { awk -v a="$1" -v b="$(clock)" -v s="$2" 'BEGIN { exit !(b - a >= s) }'; }
nap() { sleep 0.5 2>/dev/null || sleep 1; }

# Checks

# version_lt <a> <b>: a is older than b (dotted versions; missing parts count as 0).
version_lt() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		n = split(a, x, "."); m = split(b, y, ".")
		for (i = 1; i <= 3; i++) {
			xi = (i <= n) ? x[i] + 0 : 0; yi = (i <= m) ? y[i] + 0 : 0
			if (xi != yi) exit !(xi < yi)
		}
		exit 1
	}'
}

# port_in_use <port>: something listens on it, on any address (/proc/net/tcp*, or curl where there is no /proc).
port_in_use() {
	if [ -r /proc/net/tcp ]; then
		hex=$(printf ':%04X' "$1")
		for table in /proc/net/tcp /proc/net/tcp6; do
			[ -r "$table" ] || continue
			if awk -v p="$hex" 'NR > 1 && $4 == "0A" && substr($2, length($2) - 4) == p { found = 1 }
				END { exit !found }' "$table"; then return 0; fi
		done
		return 1
	fi
	# curl exits with 7 when the connection is refused, so the port is free.
	curl_status=0
	curl --noproxy '*' -s -o /dev/null --max-time 2 "http://127.0.0.1:$1/" || curl_status=$?
	[ "$curl_status" -ne 7 ]
}

check_port_free() {
	if port_in_use "$port"; then die "port $port is in use. Stop it, or pick another port with --port <n>."; fi
}

# The demo runs on the committed dev defaults only: every build embeds a local override file (local.env or
# cloud.env, or a stray copy of one), and such a file may hold real values.
check_no_override() {
	found=''
	for file in "$repo"/internal/config/local.env* "$repo"/internal/config/cloud.env*; do
		[ -e "$file" ] || continue
		case ${file##*/} in
		local.env.defaults | local.env.example | cloud.env.defaults | cloud.env.example) ;;
		*) found="$found${found:+, }${file##*/}" ;;
		esac
	done
	[ -z "$found" ] || die "internal/config holds $found. Every build embeds such an override file, which may hold" \
		"real values, and the demo runs on the committed dev defaults only: move it out of the checkout, or run the" \
		"demo from a clean clone."
}

# go: as new as go.mod asks, or allowed to fetch that toolchain (GOTOOLCHAIN auto, the default). A development
# build of Go has no release number to compare and is taken as it is.
check_go() {
	go_want=$(sed -n 's/^go \([0-9][0-9.]*\).*/\1/p' go.mod | head -n 1)
	go_hint="Install Go $go_want or newer (https://go.dev/dl/). An older Go (1.21 or newer) works too while"
	go_hint="$go_hint GOTOOLCHAIN allows the automatic toolchain (auto, the default): it then fetches Go $go_want by"
	go_hint="$go_hint itself, which needs the network once."
	command -v go >/dev/null 2>&1 || die "go wasn't found on PATH. $go_hint"
	# Asked outside the module, so the go command reports itself instead of switching toolchains.
	go_version=$(cd / && go version | sed 's/^go version //; s| [^ ]*/[^ ]*$||')
	go_mode=$(cd / && go env GOTOOLCHAIN) || {
		go_mode=''
		warn "'go env GOTOOLCHAIN' failed (see above); the Go version check assumes no automatic toolchain"
	}
	case $go_version in go[0-9]*) go_release=${go_version#go} ;; *) go_release='' ;; esac
	if [ -n "$go_release" ] && [ -n "$go_want" ] && version_lt "$go_release" "$go_want"; then
		case $go_mode in
		*auto*) warn "Go $go_release is older than the go $go_want that go.mod asks for: the go command fetches" \
			"Go $go_want on the first build (GOTOOLCHAIN=$go_mode), which needs the network once" ;;
		*) die "Go $go_release is older than the go $go_want that go.mod asks for, and" \
			"GOTOOLCHAIN=${go_mode:-(unknown)} doesn't allow the automatic toolchain. $go_hint" ;;
		esac
	fi
}

# node and npm, node matching the dashboard's engines.node range (||, ^, ~, >=, >, <=, <, = and x-ranges). The node
# script prints the range and exits with 0 on a match, 1 on a mismatch and 2 when it can't read the range; without a
# range it prints nothing and exits with 0.
check_node() {
	command -v node >/dev/null 2>&1 || die "node wasn't found on PATH: the dashboard build needs Node.js with npm" \
		"(https://nodejs.org)."
	command -v npm >/dev/null 2>&1 || die "npm wasn't found on PATH: the dashboard build needs Node.js with npm" \
		"(https://nodejs.org)."
	node_version=$(node --version)
	node_version=${node_version#v}
	npm_version=$(npm --version)
	engines_status=0
	engines_range=$(node -e '
process.on("uncaughtException", () => process.exit(2));
const pkg = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"));
const range = String((pkg.engines || {}).node || "").trim();
if (!range) process.exit(0);
console.log(range);
const version = process.versions.node.split(".").map(Number);
const compare = (a, b) => {
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  return 0;
};
// One comparator: an operator and a version whose missing or x parts are wildcards.
const comparator = /^(\^|~|>=|<=|>|<|=)?v?([0-9xX*.]+)$/;
const satisfies = (text) => {
  const [, op = "=", spec] = comparator.exec(text);
  const parts = spec.split(".").map((s) => (/^\d+$/.test(s) ? Number(s) : null));
  let given = 0;
  while (given < 3 && parts[given] != null) given++;
  const low = [0, 1, 2].map((i) => (i < given ? parts[i] : 0));
  const exact = given === 3;
  // The first version past the given parts: "22" -> 23.0.0, "22.13" or "22.13.1" -> 22.14.0; none for "*".
  const next = given === 0 ? null : given === 1 ? [low[0] + 1, 0, 0] : [low[0], low[1] + 1, 0];
  switch (op) {
    case ">=": return compare(version, low) >= 0;
    case "<": return compare(version, low) < 0;
    case ">": return exact ? compare(version, low) > 0 : next !== null && compare(version, next) >= 0;
    case "<=": return exact ? compare(version, low) <= 0 : next === null || compare(version, next) < 0;
    case "~": return given === 0 || (compare(version, low) >= 0 && compare(version, next) < 0);
    case "^": {
      if (given === 0) return true;
      const upper = low[0] > 0 || given === 1 ? [low[0] + 1, 0, 0]
        : low[1] > 0 || given === 2 ? [0, low[1] + 1, 0]
        : [0, 0, low[2] + 1];
      return compare(version, low) >= 0 && compare(version, upper) < 0;
    }
    default:
      if (exact) return compare(version, low) === 0;
      return next === null || (compare(version, low) >= 0 && compare(version, next) < 0);
  }
};
const alternatives = range.split("||").map((set) => set.trim().split(/\s+/).filter(Boolean));
if (!alternatives.every((set) => set.length > 0 && set.every((c) => comparator.test(c)))) process.exit(2);
process.exit(alternatives.some((set) => set.every(satisfies)) ? 0 : 1);
' "$frontend_dir/package.json" 2>/dev/null) || engines_status=$?
	case $engines_status in
	0) ;;
	1) die "node $node_version doesn't match the dashboard's engines (node $engines_range): install a matching" \
		"Node.js (https://nodejs.org)." ;;
	*) warn "couldn't read the dashboard's engines range for node ('$engines_range');" \
		"going on with node $node_version" ;;
	esac
}

check_tools() {
	check_go
	check_node
	command -v curl >/dev/null 2>&1 || die "curl wasn't found on PATH: the script checks the server's /health with it."
	step "tools: $go_version (go.mod asks for $go_want or newer)," \
		"node $node_version${engines_range:+ (dashboard engines: node $engines_range)}, npm $npm_version"
}

# Steps

build_dashboard() {
	started=$(clock)
	step "dashboard: npm ci (in $frontend_dir)"
	(cd "$frontend_dir" && npm ci --prefer-offline --no-audit --no-fund) || die 'npm ci failed; see its output above.'
	step 'dashboard: npm run build'
	(cd "$frontend_dir" && npm run build) || die 'npm run build failed; see its output above.'
	[ -f "$frontend_dir/dist/index.html" ] || die "npm run build left no dist/index.html in $frontend_dir"
	# client/dist keeps its tracked .gitkeep placeholder (written back when missing); the rest is the new build.
	mkdir -p client/dist
	for file in client/dist/* client/dist/.[!.]* client/dist/..?*; do
		[ -e "$file" ] || [ -L "$file" ] || continue
		[ "$file" = client/dist/.gitkeep ] || rm -rf -- "$file"
	done
	cp -R -- "$frontend_dir/dist/." client/dist/
	[ -f client/dist/.gitkeep ] || say 'The dashboard build is copied here by make frontend.' >client/dist/.gitkeep
	files=$(find client/dist -type f ! -name .gitkeep | wc -l)
	step "dashboard: dist/ copied into client/dist ($((files)) files; $(since "$started" 0) s)"
}

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

# go build -tags cloud with the dev stamp. A port other than the default reaches the build through an overlay that
# adds internal/config/cloud.env from a temporary folder; the folder is deleted right after the build.
build_server() {
	started=$(clock)
	mkdir -p tmp
	# The go flags collect in the function's positional parameters, the one list POSIX sh has.
	set -- -tags cloud -ldflags "-X clinic-api/internal/buildmode.Version=$stamp" -o "$bin"
	settings='the committed dev defaults'
	if [ "$port" != "$default_port" ]; then
		overlay_dir=$(mktemp -d "${TMPDIR:-/tmp}/zeal-demo-cloud.XXXXXX")
		printf 'PORT=%s\n' "$port" >"$overlay_dir/cloud.env"
		printf '{"Replace":{"%s":"%s"}}\n' "$(json_escape "$repo/internal/config/cloud.env")" \
			"$(json_escape "$overlay_dir/cloud.env")" >"$overlay_dir/overlay.json"
		set -- "$@" -overlay "$overlay_dir/overlay.json"
		settings="the dev defaults plus PORT=$port (a build overlay adds internal/config/cloud.env from a temporary"
		settings="$settings file; nothing is written into the checkout)"
	fi
	step "build: $bin_name (cloud node, version $stamp) on $settings"
	go build "$@" ./cmd/server || die 'go build failed; see its output above.'
	if [ -n "$overlay_dir" ]; then
		rm -rf -- "$overlay_dir"
		overlay_dir=''
	fi
	step "build: $bin ($(since "$started" 0) s)"
}

# --dev --seed-only --demo in the data folder: migrations, then the demo data. The demo data is versioned like the
# migrations, so on kept data this changes nothing, finishes a seed that was cut short or adds new demo steps.
seed_demo_data() {
	if [ "$reset" = 1 ] && [ -e "$data_root" ]; then
		step "reset: deleting $data_root"
		rm -rf -- "$data_root"
	fi
	if [ -f "$data_root/data/clinic.db" ]; then seeded='kept the existing demo data'; else seeded='new demo data'; fi
	mkdir -p "$data_root"
	step "seed: $bin_name --dev --seed-only --demo (in $data_root)"
	started=$(clock)
	seed_status=0
	(cd "$data_root" && exec "$bin" --dev --seed-only --demo) >"$seed_log" 2>&1 </dev/null || seed_status=$?
	[ "$seed_status" = 0 ] || die_log "$seed_log" "the seed failed (exit $seed_status)"
	step "seed: $seeded ($(since "$started" 1) s)"
}

# health_ok: /health answers ok as this build (another version on the port is an error).
health_ok() {
	health=$(curl --noproxy '*' -fsS --max-time 3 "$loopback_url/health" 2>/dev/null) || return 1
	case $health in *'"status":"ok"'*) ;; *) return 1 ;; esac
	health_version=$(printf '%s' "$health" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
	[ "$health_version" = "$stamp" ] ||
		die "port $port answers /health as version '$health_version', not this build ($stamp)"
}

start_server() {
	check_port_free # again: the builds took a while
	server_args="--dev"
	if [ "$lan" = 1 ]; then
		server_args="--dev --lan"
	fi
	step "start: cloud node, $bin_name $server_args on port $port (in $data_root)"
	started=$(clock)
	# SIGHUP ignored: a closed terminal reaches the server only through the script's trap, as a clean stop.
	(trap '' HUP && cd "$data_root" && exec "$bin" $server_args) >"$console_log" 2>&1 </dev/null &
	server_pid=$!
	until health_ok; do
		if ! kill -0 "$server_pid" 2>/dev/null; then
			start_status=0
			wait "$server_pid" || start_status=$?
			server_pid=''
			die_log "$console_log" "the cloud node exited while starting (exit $start_status)"
		fi
		if passed "$started" "$health_timeout_s"; then
			die_log "$console_log" "the cloud node didn't answer /health within $health_timeout_s s"
		fi
		nap
	done
	step "start: cloud node healthy on $loopback_url ($(since "$started" 1) s)"
}

show_ready() {
	say ''
	say "Demo ready: $url"
	ready=1
	say "Sign in with the password $demo_password as:"
	for user in $demo_users; do say "$(printf '  %-9s %s' "${user%%:*}" "${user#*:}")"; done
	say 'Financial writes (invoices, payments) are read-only here: a cloud node accepts them only while its' \
		'clinic is online and synced.'
	say "Data: $data_root (kept between runs; --reset starts over)"
	say "Logs: $console_log"
	say 'Press Ctrl+C to stop the demo.'
	say ''
	[ "$no_browser" = 1 ] || open_browser "$url/"
}

open_browser() {
	if command -v xdg-open >/dev/null 2>&1 && [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
		(xdg-open "$1" >/dev/null 2>&1 &)
	else
		warn "couldn't open a browser (no xdg-open or no display); open $1 yourself"
	fi
}

# Waits for the server. It returns only when the server stopped on its own: a stop request ends in on_signal.
wait_for_server() {
	wait_status=0
	wait "$server_pid" || wait_status=$?
	server_pid=''
	die_log "$console_log" "the cloud node stopped on its own (exit $wait_status)"
}

# Stopping

# stop_server <INT|TERM>: sends the server that signal and waits for it (SIGTERM after stop_grace_s of SIGINT,
# SIGKILL after stop_kill_s of SIGTERM). Returns the server's exit status.
stop_server() {
	signal=$1
	kill -s "$signal" "$server_pid" 2>/dev/null || true
	sent=$(clock)
	while kill -0 "$server_pid" 2>/dev/null; do
		if [ "$signal" = INT ] && passed "$sent" "$stop_grace_s"; then
			warn "the cloud node didn't stop within $stop_grace_s s of SIGINT; sending SIGTERM"
			signal=TERM
			kill -s TERM "$server_pid" 2>/dev/null || true
			sent=$(clock)
		elif [ "$signal" = TERM ] && passed "$sent" "$stop_kill_s"; then
			warn "the cloud node didn't stop within $stop_kill_s s of SIGTERM; stopping it hard (not a clean shutdown)"
			kill -s KILL "$server_pid" 2>/dev/null || true
			break
		fi
		nap
	done
	stop_status=0
	wait "$server_pid" 2>/dev/null || stop_status=$?
	server_pid=''
	return "$stop_status"
}

# The SIGINT, SIGTERM and SIGHUP trap; $1 is the exit status for a stop before the ready line.
on_signal() {
	[ "$stopping" = 0 ] || return 0
	stopping=1
	if [ "$ready" = 0 ]; then
		# The server hasn't served yet: SIGTERM ends it, at once or cleanly.
		[ -z "$server_pid" ] || stop_server TERM || true
		say 'Stopped before the demo was ready.'
		exit "$1"
	fi
	step 'stopping'
	server_status=0
	stop_server INT || server_status=$?
	if [ "$server_status" = 0 ] && grep -q 'Server shutting down' "$console_log" 2>/dev/null; then
		step 'stop: cloud node stopped cleanly (exit 0; its log shows the shutdown)'
		say 'Demo stopped.'
		exit 0
	fi
	die_log "$console_log" "the cloud node didn't stop cleanly (exit $server_status)"
}

# The EXIT trap: an error leaves no overlay folder and no server behind.
cleanup() {
	stopping=1
	[ -z "$overlay_dir" ] || rm -rf -- "$overlay_dir"
	[ -z "$server_pid" ] || stop_server TERM || true
}

# Main

frontend=''
port=''
lan=0
reset=0
no_browser=0
while [ $# -gt 0 ]; do
	case $1 in
	--frontend=*) frontend=${1#*=} ;;
	--port=*) port=${1#*=} ;;
	--frontend | --port)
		[ $# -ge 2 ] || die "$1 needs a value"
		if [ "$1" = --frontend ]; then frontend=$2; else port=$2; fi
		shift
		;;
	--lan) lan=1 ;;
	--reset) reset=1 ;;
	--no-browser) no_browser=1 ;;
	-h | --help) usage && exit 0 ;;
	*) usage >&2 && die "unknown option: $1" ;;
	esac
	shift
done

case $(uname -s) in
MINGW* | MSYS* | CYGWIN*) die 'this script runs the cloud node on Linux. On Windows, run pwsh scripts/demo.ps1 or' \
	'scripts/demo-two-node.ps1.' ;;
esac
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(dirname -- "$script_dir")
[ -f "$repo/go.mod" ] && [ -d "$repo/cmd/server" ] || die "can't find the backend checkout next to $script_dir"

default_port=$(tr -d '\r' <"$repo/$defaults_file" | sed -n 's/^ *PORT *= *"\{0,1\}\([0-9][0-9]*\)"\{0,1\} *$/\1/p' |
	head -n 1)
[ -n "$default_port" ] || die "no PORT in $defaults_file"
[ -n "$port" ] || port=$default_port
case $port in '' | 0* | *[!0-9]*) die "--port $port isn't a usable port (1024-65535)." ;; esac
[ "$port" -ge 1024 ] && [ "$port" -le 65535 ] || die "--port $port isn't a usable port (1024-65535)."

# A relative --frontend is relative to the current folder, so it's resolved before the script moves.
default_frontend="$(dirname -- "$repo")/zeal-clinic-frontend"
[ -n "$frontend" ] || frontend=$default_frontend
frontend_dir=$(CDPATH='' cd -- "$frontend" 2>/dev/null && pwd) && [ -f "$frontend_dir/package.json" ] ||
	die "no dashboard checkout at $frontend. By default the demo builds the dashboard from ../zeal-clinic-frontend" \
		"next to this checkout ($default_frontend); clone it there, or pass --frontend <path> to use another folder."

cd "$repo"
version=$(tr -d ' \r\n' <VERSION)
case $version in '' | [!0-9A-Za-z]* | *[!0-9A-Za-z.+-]*) die "VERSION holds an unexpected value: '$version'" ;; esac
stamp="$version-dev"
bin="$repo/tmp/$bin_name"
data_root="$repo/tmp/demo/cloud"
seed_log="$data_root/seed-console.log"
console_log="$data_root/server-console.log"
loopback_url="http://127.0.0.1:$port" # where the script checks /health
if [ "$lan" = 1 ]; then url="http://$(get_lan_ip):$port"; else url=$loopback_url; fi

trap cleanup EXIT
trap 'on_signal 129' HUP
trap 'on_signal 130' INT
trap 'on_signal 143' TERM

check_no_override
check_port_free
check_tools
build_dashboard
build_server
seed_demo_data
start_server
show_ready
wait_for_server
