package main

import (
	"path/filepath"
	"sort"
)

func listACMPorts() []string {
	matches, err := filepath.Glob("/dev/ttyACM*")
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}
