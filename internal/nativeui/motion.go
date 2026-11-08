package nativeui

import "time"

// The four lengths a small piece of the interface may take to change. Callers
// name the one that fits and never write a duration of their own.
const (
	// motionColor eases a face between rest, hover and selection.
	motionColor = 120 * time.Millisecond
	// motionRow is a row or small item arriving, leaving or turning (a chevron).
	motionRow = 140 * time.Millisecond
	// motionPanel is a chip, card or panel growing, shrinking or sliding.
	motionPanel = 160 * time.Millisecond
	// motionHandOver is one shape giving way to the next (the status slot).
	motionHandOver = 180 * time.Millisecond
)
