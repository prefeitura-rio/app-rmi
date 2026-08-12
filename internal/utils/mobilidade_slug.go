package utils

import (
	"regexp"
	"strings"
	"unicode"
)

var mobilidadeNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// MobilidadeSlugify converts a catalog label to a stable lowercase slug (seed script parity).
func MobilidadeSlugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	return strings.Trim(mobilidadeNonAlnum.ReplaceAllString(b.String(), "_"), "_")
}

// MobilidadeBrandIDFromName returns brand_<slug> for a catalog brand name.
func MobilidadeBrandIDFromName(name string) string {
	return "brand_" + MobilidadeSlugify(name)
}

// MobilidadeModelIDFromBrandAndName returns model_<brandSlug>_<modelSlug>.
func MobilidadeModelIDFromBrandAndName(brandName, modelName string) string {
	return "model_" + MobilidadeSlugify(brandName) + "_" + MobilidadeSlugify(modelName)
}
