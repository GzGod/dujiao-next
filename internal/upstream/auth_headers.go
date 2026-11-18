package upstream

import "net/http"

const (
	HeaderXuegaoAPIKey    = "XuegaoAI-Api-Key"
	HeaderXuegaoTimestamp = "XuegaoAI-Timestamp"
	HeaderXuegaoSignature = "XuegaoAI-Signature"
)

// ReadAuthHeaders accepts either complete header family, never a mixed or
// conflicting identity. Legacy names remain for third-party upstream services.
func ReadAuthHeaders(h http.Header) (string, string, string) {
	branded := []string{HeaderXuegaoAPIKey, HeaderXuegaoTimestamp, HeaderXuegaoSignature}
	legacy := []string{HeaderApiKey, HeaderTimestamp, HeaderSignature}
	hasBranded := false
	for _, name := range branded {
		if len(h.Values(name)) > 0 {
			hasBranded = true
		}
	}
	names := legacy
	if hasBranded {
		names = branded
	}
	values := make([]string, 3)
	for i, name := range names {
		if len(h.Values(name)) != 1 || h.Get(name) == "" {
			return "", "", ""
		}
		values[i] = h.Get(name)
		if hasBranded && len(h.Values(legacy[i])) > 0 {
			if len(h.Values(legacy[i])) != 1 || h.Get(legacy[i]) != values[i] {
				return "", "", ""
			}
		}
	}
	return values[0], values[1], values[2]
}
