//go:build cloud

// Package buildmode exposes build-tag-driven constants so any module can
// branch on "am I the cloud binary?" without each call site reinventing the
// check (e.g. proxying through env vars like PEER_URL).
package buildmode

// Cloud is true when this binary was compiled with `-tags cloud`. False
// otherwise. Use it for behavioural switches that should follow the binary
// rather than runtime config, e.g. whether sync.Apply treats local writes
// as authoritative.
const Cloud = true
