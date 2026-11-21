//go:build !windows

package main

func canvasWheelNotches(dy float64, precise bool) float64 {
	// On macOS MyGo delivers 40 DIPs per wheel line and individual pixels
	// from a trackpad. Preserve fractional travel for fine adjustments.
	if precise {
		return dy / 100
	}
	return dy / 40
}
