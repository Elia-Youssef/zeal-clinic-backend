// Package envfile reads the config's env files and holds the rules for their
// secrets. Each build has committed dev defaults (<name>.defaults) and an
// optional git-ignored local override (<name>) whose non-empty values replace
// the defaults key by key. The package embeds nothing, so the pre-build check
// (cmd/releasecheck) never compiles the values it checks into a binary.
package envfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// Keys of the env files.
const (
	KeyPort            = "PORT"
	KeyJWTSecret       = "JWT_SECRET"
	KeyJWTLifetime     = "JWT_LIFETIME"
	KeyPeerURL         = "PEER_URL"
	KeySyncSecret      = "SYNC_SECRET"
	KeyPublicURL       = "PUBLIC_URL"
	KeyPublishSecret   = "PUBLISH_SECRET"
	KeyDBEncryptionKey = "DB_ENCRYPTION_KEY"
)

// DefaultsSuffix names the committed dev defaults of an override file:
// local.env.defaults for local.env.
const DefaultsSuffix = ".defaults"

// SecretKeys are the keys whose committed defaults are dev values.
var SecretKeys = []string{KeyJWTSecret, KeySyncSecret, KeyPublishSecret, KeyDBEncryptionKey}

// DevMarker marks a dev value; the committed defaults of every secret but the
// DB key start with it.
const DevMarker = "dev-only"

// MinJWTSecretLen is the shortest JWT secret a release build accepts.
const MinJWTSecretLen = 32

// Read parses one env file. A syntax error is reported without the parser's
// message, which can quote the file's content.
func Read(fsys fs.FS, name string) (map[string]string, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	env, err := godotenv.UnmarshalBytes(b)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid env file syntax", name)
	}
	return env, nil
}

// Load reads name+".defaults" from fsys and applies the non-empty values of
// the override file name, when it exists, key by key. It returns the merged
// values and the keys the override set.
func Load(fsys fs.FS, name string) (map[string]string, []string, error) {
	env, err := Read(fsys, name+DefaultsSuffix)
	if err != nil {
		return nil, nil, err
	}
	override, err := Read(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return env, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var set []string
	for k, v := range override {
		if v == "" {
			continue
		}
		env[k] = v
		set = append(set, k)
	}
	sort.Strings(set)
	return env, set, nil
}

// Problems lists what makes the secrets in env unusable: a DB key that isn't
// 64 hex characters for any build and, for a release, also a dev value (see
// DevKeys) and a JWT secret shorter than MinJWTSecretLen. The messages name
// keys, never values.
func Problems(env, defaults map[string]string, release bool) []string {
	var out []string
	if !ValidDBKey(env[KeyDBEncryptionKey]) {
		out = append(out, KeyDBEncryptionKey+" must be 64 hex characters")
	}
	if !release {
		return out
	}
	for _, k := range DevKeys(env, defaults) {
		out = append(out, k+" holds a dev value")
	}
	if len(env[KeyJWTSecret]) < MinJWTSecretLen {
		out = append(out, fmt.Sprintf("%s must be at least %d characters", KeyJWTSecret, MinJWTSecretLen))
	}
	return out
}

// DevKeys lists the secret keys of env that hold a dev value: the committed
// default of the same key, or any value marked dev-only.
func DevKeys(env, defaults map[string]string) []string {
	var out []string
	for _, k := range SecretKeys {
		v := env[k]
		if v == "" {
			continue
		}
		if (defaults[k] != "" && strings.EqualFold(v, defaults[k])) || strings.Contains(strings.ToLower(v), DevMarker) {
			out = append(out, k)
		}
	}
	return out
}

// ValidDBKey reports whether s is a database key: 64 hex characters.
func ValidDBKey(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// nodeFiles maps each node to the override file its build embeds.
var nodeFiles = map[string]string{"clinic": "local.env", "cloud": "cloud.env"}

// CheckRelease is the pre-build check of a release, run on the env files in
// dir (internal/config) before building the given nodes (clinic, cloud). For
// each node, the override file must exist and, applied over the node's
// committed defaults, hold no dev value, a JWT secret of 32 characters or more
// and a 64-hex DB key. When the clinic and cloud overrides both exist, they
// must carry the same SYNC_SECRET and DB_ENCRYPTION_KEY. It returns the
// problems found, naming files and keys, never values; the error is for a
// check that could not run.
func CheckRelease(dir string, nodes []string) ([]string, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no node to check (clinic, cloud)")
	}
	fsys := os.DirFS(dir)
	var out []string
	for _, n := range nodes {
		file, ok := nodeFiles[n]
		if !ok {
			return nil, fmt.Errorf("unknown node %q (clinic, cloud)", n)
		}
		if _, err := fs.Stat(fsys, file); errors.Is(err, fs.ErrNotExist) {
			out = append(out, fmt.Sprintf("%s: missing; the %s build needs it (start from %s.example)", file, n, file))
			continue
		}
		env, _, err := Load(fsys, file)
		if err != nil {
			return nil, err
		}
		defaults, err := Read(fsys, file+DefaultsSuffix)
		if err != nil {
			return nil, err
		}
		for _, p := range Problems(env, defaults, true) {
			out = append(out, file+": "+p)
		}
	}

	// The two nodes sync with one secret and restore each other's databases.
	clinic, err := readOverride(fsys, "clinic")
	if err != nil {
		return nil, err
	}
	cloud, err := readOverride(fsys, "cloud")
	if err != nil {
		return nil, err
	}
	if clinic != nil && cloud != nil {
		if clinic[KeySyncSecret] != cloud[KeySyncSecret] {
			out = append(out, KeySyncSecret+" differs between local.env and cloud.env")
		}
		if !strings.EqualFold(clinic[KeyDBEncryptionKey], cloud[KeyDBEncryptionKey]) {
			out = append(out, KeyDBEncryptionKey+" differs between local.env and cloud.env")
		}
	}
	return out, nil
}

// readOverride returns a node's override applied over its defaults, or nil
// when the override doesn't exist.
func readOverride(fsys fs.FS, node string) (map[string]string, error) {
	file := nodeFiles[node]
	if _, err := fs.Stat(fsys, file); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	env, _, err := Load(fsys, file)
	return env, err
}
