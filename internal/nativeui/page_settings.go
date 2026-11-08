package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// settingsPage is the one layout owner for Settings. The title bar names the
// open section, so the page is the scroll viewport and each section renders
// only controls.
const settingsContentMaxWidth float32 = 760

// settingSwitch is a labeled on/off preference: it reports whether the user
// flipped it this frame and the new value.
func settingSwitch(c *ui.Context, label string, on bool) (bool, bool) {
	value := on
	changed := ui.Switch(c, &value).Label(label).Changed()
	return value, changed
}

var appearanceOptions = []string{"System", "Light", "Dark"}

func appearanceIndex(value string) int {
	switch value {
	case "light":
		return 1
	case "dark":
		return 2
	default:
		return 0
	}
}

func appearanceFromIndex(index int) string {
	if index < 0 || index >= len(appearanceOptions) {
		return "system"
	}
	return strings.ToLower(appearanceOptions[index])
}
