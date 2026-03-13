package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SerialPortInfo struct {
	Status      string
	Name        string
	DeviceID    string
	Description string
	Caption     string
}

func getPort() (*[]SerialPortInfo, error) {
	ports := make([]SerialPortInfo, 0, 8)
	seen := make(map[string]struct{})

	add := func(deviceID, name, description string) {
		if deviceID == "" {
			return
		}
		if _, exists := seen[deviceID]; exists {
			return
		}
		seen[deviceID] = struct{}{}

		if name == "" {
			name = filepath.Base(deviceID)
		}
		if description == "" {
			description = name
		}

		ports = append(ports, SerialPortInfo{
			Status:      "OK",
			Name:        name,
			DeviceID:    deviceID,
			Description: description,
			Caption:     description,
		})
	}

	// Linux/Fedora: USBシリアルは /dev/serial/by-id が最も識別しやすい。
	if entries, err := os.ReadDir("/dev/serial/by-id"); err == nil {
		for _, e := range entries {
			linkPath := filepath.Join("/dev/serial/by-id", e.Name())
			resolved, err := filepath.EvalSymlinks(linkPath)
			if err != nil || !strings.HasPrefix(resolved, "/dev/tty") {
				continue
			}
			add(resolved, e.Name(), linkPath)
		}
	}

	// by-id が無い環境向けフォールバック。
	patterns := []string{
		"/dev/ttyACM*",
		"/dev/ttyUSB*",
		"/dev/ttyAMA*",
		"/dev/ttyXRUSB*",
		"/dev/ttyS*",
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, m := range matches {
			add(m, filepath.Base(m), "serial device")
		}
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].DeviceID < ports[j].DeviceID
	})

	return &ports, nil
}
