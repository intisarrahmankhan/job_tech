package dedup

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Clean trims whitespace. Intended for user-facing fields.
func Clean(s string) string {
	return strings.TrimSpace(s)
}

// NormalizeCompany lowercases + trims. Company names are treated
// case-insensitively for deduplication (e.g. "Pathao" == "pathao").
func NormalizeCompany(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// SourceHash produces a deterministic SHA256 *similarity* fingerprint
// used as the unique dedup key in Postgres. It deliberately ignores the
// posting URL so that the SAME role posted to multiple boards
// (e.g. LinkedIn + BDJobs) collides into one record — the multi-source
// merge logic in handlers.IngestJob then appends the new source.
func SourceHash(company, title string) string {
	h := sha256.New()
	h.Write([]byte(NormalizeCompany(company)))
	h.Write([]byte("|"))
	h.Write([]byte(strings.ToLower(strings.TrimSpace(title))))
	return hex.EncodeToString(h.Sum(nil))
}
