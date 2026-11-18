package upstream

import (
	"net/http"
	"testing"
)

func TestReadAuthHeadersFamilies(t *testing.T) {
	for _, tc := range []struct {
		name            string
		branded, legacy []string
		valid           bool
	}{
		{"branded", []string{"key", "123", "sig"}, nil, true},
		{"legacy", nil, []string{"key", "123", "sig"}, true},
		{"both", []string{"key", "123", "sig"}, []string{"key", "123", "sig"}, true},
		{"mixed", []string{"key", "", ""}, []string{"", "123", "sig"}, false},
		{"conflicting key", []string{"key", "123", "sig"}, []string{"other", "123", "sig"}, false},
		{"conflicting signature", []string{"key", "123", "sig"}, []string{"key", "123", "other"}, false},
		{"empty", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for i, v := range tc.branded {
				if v != "" {
					h.Set([]string{HeaderXuegaoAPIKey, HeaderXuegaoTimestamp, HeaderXuegaoSignature}[i], v)
				}
			}
			for i, v := range tc.legacy {
				if v != "" {
					h.Set([]string{HeaderApiKey, HeaderTimestamp, HeaderSignature}[i], v)
				}
			}
			k, ts, sig := ReadAuthHeaders(h)
			if tc.valid {
				if k != "key" || ts != "123" || sig != "sig" {
					t.Fatal("valid family rejected")
				}
			} else if k != "" || ts != "" || sig != "" {
				t.Fatal("ambiguous family accepted")
			}
		})
	}
}

func TestReadAuthHeadersRejectsDuplicateValues(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderXuegaoAPIKey, "key")
	h.Set(HeaderXuegaoTimestamp, "123")
	h.Set(HeaderXuegaoSignature, "sig")
	h.Add(HeaderXuegaoAPIKey, "other")
	k, _, _ := ReadAuthHeaders(h)
	if k != "" {
		t.Fatal("duplicate identity accepted")
	}
}
