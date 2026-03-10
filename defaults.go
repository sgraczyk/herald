// Package herald provides embedded assets for the Herald bot.
package herald

import _ "embed"

// DefaultConfig holds the embedded default configuration used when no config
// file is found on disk.
//
//go:embed config.json.example
var DefaultConfig []byte
