package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	songChunkBytes = 32 // 16 notes x (midi,len)
	songSlotCount  = 4
	songWaitMargin = 20 * time.Millisecond
)

type SongPlayStats struct {
	Chunks  int
	Notes   int
	Elapsed time.Duration
}

func (r *Roomba) PlaySongFile(path string) (SongPlayStats, error) {
	data, err := loadSongDataFile(path)
	if err != nil {
		return SongPlayStats{}, err
	}
	return r.PlaySongData(data)
}

func (r *Roomba) PlaySongData(data []byte) (SongPlayStats, error) {
	var stats SongPlayStats

	if len(data) == 0 {
		return stats, fmt.Errorf("empty song data")
	}
	if len(data)%2 != 0 {
		// [midi][len] ペアを維持するため、末尾の不完全データは捨てる。
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return stats, fmt.Errorf("song data has no complete midi/len pair")
	}

	chunks := splitSongChunks(data, songChunkBytes)
	if len(chunks) == 0 {
		return stats, fmt.Errorf("no playable chunks")
	}

	startedAt := time.Now()
	for i, chunk := range chunks {
		slot := byte(i % songSlotCount)
		if err := r.DefineSong(slot, chunk); err != nil {
			return stats, fmt.Errorf("define song failed at chunk %d/%d: %w", i+1, len(chunks), err)
		}
		if err := r.PlaySong(slot); err != nil {
			return stats, fmt.Errorf("play song failed at chunk %d/%d: %w", i+1, len(chunks), err)
		}
		time.Sleep(songPlaybackDuration(chunk) + songWaitMargin)
	}

	stats.Chunks = len(chunks)
	stats.Notes = len(data) / 2
	stats.Elapsed = time.Since(startedAt)
	return stats, nil
}

func splitSongChunks(data []byte, chunkBytes int) [][]byte {
	if chunkBytes <= 0 {
		return nil
	}

	chunks := make([][]byte, 0, (len(data)+chunkBytes-1)/chunkBytes)
	for start := 0; start < len(data); start += chunkBytes {
		end := start + chunkBytes
		if end > len(data) {
			end = len(data)
		}
		chunk := data[start:end]
		if len(chunk)%2 != 0 {
			chunk = chunk[:len(chunk)-1]
		}
		if len(chunk) == 0 {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func songPlaybackDuration(song []byte) time.Duration {
	totalTicks := 0
	for i := 1; i < len(song); i += 2 {
		totalTicks += int(song[i])
	}
	// Roomba OI の duration は 1/64 秒単位。
	return time.Second * time.Duration(totalTicks) / 64
}

func loadSongDataFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read song file: %w", err)
	}

	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("song file is empty")
	}

	if isLikelySongText(raw) {
		data, err := parseSongText(raw)
		if err != nil {
			return nil, fmt.Errorf("parse song text: %w", err)
		}
		return data, nil
	}

	return append([]byte(nil), raw...), nil
}

func isLikelySongText(raw []byte) bool {
	for _, b := range raw {
		if b >= '0' && b <= '9' {
			continue
		}
		switch b {
		case ' ', '\t', '\n', '\r', ',', '[', ']':
			continue
		default:
			return false
		}
	}
	return true
}

func parseSongText(raw []byte) ([]byte, error) {
	replacer := strings.NewReplacer(
		",", " ",
		"[", " ",
		"]", " ",
	)
	normalized := replacer.Replace(string(raw))
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return nil, fmt.Errorf("no values found")
	}

	data := make([]byte, 0, len(fields))
	for _, token := range fields {
		v, err := strconv.Atoi(token)
		if err != nil {
			return nil, fmt.Errorf("invalid token: %q", token)
		}
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("value out of range (0-255): %d", v)
		}
		data = append(data, byte(v))
	}

	return data, nil
}
