package dedup

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"applysmart/internal/provider"
)

// fieldSeparator keeps field boundaries unambiguous, so that e.g. title "ab"
// with company "c" does not hash the same as title "a" with company "bc".
const fieldSeparator = "\x1f"

// GenerateContentHash returns the hex-encoded SHA-256 of the job's normalised
// title, company and description.
func GenerateContentHash(job provider.RawJob) string {
	h := sha256.New()
	for i, field := range []string{job.Title, job.Company, job.Description} {
		if i > 0 {
			h.Write([]byte(fieldSeparator))
		}
		h.Write([]byte(normalize(field)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
