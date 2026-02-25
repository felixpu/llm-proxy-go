package repository

import "math"

// boolToInt converts a boolean to an integer (1 or 0) for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// roundToPlaces rounds a float to the specified number of decimal places.
func roundToPlaces(val float64, places int) float64 {
	mult := math.Pow(10, float64(places))
	return math.Round(val*mult) / mult
}
