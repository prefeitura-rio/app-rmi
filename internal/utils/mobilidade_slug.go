package utils

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var mobilidadeNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// MobilidadeSlugify converts a catalog label to a stable lowercase ASCII slug.
// Accents are folded (São Paulo → sao_paulo); separators become underscores.
func MobilidadeSlugify(s string) string {
	s = foldAccents(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	return strings.Trim(mobilidadeNonAlnum.ReplaceAllString(b.String(), "_"), "_")
}

func foldAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return out
}

// MobilidadeBrandIDFromName returns brand_<slug> for a catalog brand name.
func MobilidadeBrandIDFromName(name string) string {
	return "brand_" + MobilidadeSlugify(name)
}

// MobilidadeModelIDFromBrandAndName returns model_<brandSlug>_<modelSlug>.
func MobilidadeModelIDFromBrandAndName(brandName, modelName string) string {
	return "model_" + MobilidadeSlugify(brandName) + "_" + MobilidadeSlugify(modelName)
}
