package runtime

import "strconv"

const (
	MarkerOpen  = "<!--rill:o"
	MarkerClose = "<!--/rill:o"
	markerEnd   = "-->"
	maxMarker   = 16
)

var (
	openMarkers  = buildMarkers(MarkerOpen)
	closeMarkers = buildMarkers(MarkerClose)
)

func buildMarkers(prefix string) [][]byte {
	all := make([][]byte, maxMarker)
	for level := range maxMarker {
		all[level] = []byte(prefix + strconv.Itoa(level) + markerEnd)
	}
	return all
}

func openMarker(level int) []byte {
	if level < 0 || level >= maxMarker {
		return nil
	}
	return openMarkers[level]
}

func closeMarker(level int) []byte {
	if level < 0 || level >= maxMarker {
		return nil
	}
	return closeMarkers[level]
}
