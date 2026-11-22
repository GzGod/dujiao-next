// Package productguide holds reviewed, public buying guidance shared by HTML
// projections and the interactive storefront. Prices and stock stay live.
package productguide

import (
	_ "embed"
	"encoding/json"
)

//go:embed content.json
var content []byte

type Step struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}
type FAQ struct {
	Q string `json:"q"`
	A string `json:"a"`
}
type Guide struct {
	Summary  string   `json:"summary"`
	Suitable string   `json:"suitable"`
	Checks   []string `json:"checks"`
	Before   []string `json:"before"`
	Steps    []Step   `json:"steps"`
	FAQs     []FAQ    `json:"faqs"`
	Kind     string   `json:"kind"`
}
type product struct {
	CanonicalSlug string           `json:"canonicalSlug"`
	Kind          string           `json:"kind"`
	Locales       map[string]Guide `json:"locales"`
}
type SiteGuide struct {
	Subtitle      string   `json:"subtitle"`
	Intro         string   `json:"intro"`
	ServicesTitle string   `json:"servicesTitle"`
	Services      []string `json:"services"`
	ContactTitle  string   `json:"contactTitle"`
	ContactText   string   `json:"contactText"`
}
type catalog struct {
	Site     map[string]SiteGuide         `json:"site"`
	Labels   map[string]map[string]string `json:"labels"`
	Common   map[string]map[string]Guide  `json:"common"`
	Products map[string]product           `json:"products"`
}

var data = func() catalog {
	var d catalog
	if err := json.Unmarshal(content, &d); err != nil {
		panic(err)
	}
	return d
}()

func Get(slug, lang string) (Guide, bool) {
	p, ok := data.Products[SourceSlug(slug)]
	if !ok {
		return Guide{}, false
	}
	if _, ok := data.Labels[lang]; !ok {
		lang = "zh-CN"
	}
	g := p.Locales[lang]
	common := data.Common[p.Kind][lang]
	g.Kind = p.Kind
	g.Before = append(append([]string{}, g.Checks...), common.Before...)
	g.Steps = append([]Step{}, common.Steps...)
	g.FAQs = append(append([]FAQ{}, g.FAQs...), common.FAQs...)
	return g, true
}
func Labels(lang string) map[string]string {
	if _, ok := data.Labels[lang]; !ok {
		lang = "zh-CN"
	}
	result := map[string]string{}
	for k, v := range data.Labels[lang] {
		result[k] = v
	}
	return result
}

func Site(lang string) SiteGuide {
	g, ok := data.Site[lang]
	if !ok {
		g = data.Site["zh-CN"]
	}
	g.Services = append([]string{}, g.Services...)
	return g
}

// SourceSlug preserves existing database identifiers while allowing readable URLs.
func SourceSlug(slug string) string {
	for source, p := range data.Products {
		if p.CanonicalSlug == slug {
			return source
		}
	}
	return slug
}
func CanonicalSlug(slug string) string {
	if p, ok := data.Products[SourceSlug(slug)]; ok && p.CanonicalSlug != "" {
		return p.CanonicalSlug
	}
	return slug
}
