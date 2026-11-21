//go:build responsive

package responsive

import (
	"fmt"
	"os"

	"github.com/chromedp/cdproto/emulation"
)

type ColorScheme struct {
	Value string
}

const (
	colorSchemeFeature  = "prefers-color-scheme"
	colorSchemeVariable = "MESAIL_COLOR_SCHEME"
	lightColorScheme    = "light"
	darkColorScheme     = "dark"
)

func ChosenColorScheme() (ColorScheme, error) {
	value := os.Getenv(colorSchemeVariable)
	if value == "" {
		return ColorScheme{Value: lightColorScheme}, nil
	}
	if value != lightColorScheme && value != darkColorScheme {
		return ColorScheme{}, fmt.Errorf("%s must be %s or %s, not %q", colorSchemeVariable, lightColorScheme, darkColorScheme, value)
	}
	return ColorScheme{Value: value}, nil
}

func (scheme ColorScheme) Fill(features []*emulation.MediaFeature) []*emulation.MediaFeature {
	if setsColorScheme(features) {
		return features
	}
	filled := append([]*emulation.MediaFeature{}, features...)
	return append(filled, &emulation.MediaFeature{Name: colorSchemeFeature, Value: scheme.Value})
}

func setsColorScheme(features []*emulation.MediaFeature) bool {
	for _, feature := range features {
		if feature.Name == colorSchemeFeature {
			return true
		}
	}
	return false
}
