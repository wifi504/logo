package logo

import (
	_ "embed"
	"strings"
)

//go:embed banner.txt
var bannerTemplate string

func renderBanner() string {
	return strings.ReplaceAll(bannerTemplate, "{{VERSION}}", Version)
}
