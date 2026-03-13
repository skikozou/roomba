package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/tarm/serial"
)

type WriterMode string

const (
	WriterModeBase64 WriterMode = "base64"
	WriterModeText   WriterMode = "text"
	WriterModeHex    WriterMode = "hex"
	WriterModeInt    WriterMode = "int"
)

func Init() {
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:            true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})
}

func main() {
	Init()
	ui := NewLogUI()

	signalCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()

	go func() {
		if err := ui.Run(); err != nil {
			logrus.WithError(err).Error("UI error")
		}
		// Ctrl+CでUIが止まった場合でも確実に全体を終了させる。
		cancel()
	}()

	go func() {
		for {
			var PortName string

		selectmode:
			for {
				mode := strings.ToLower(strings.TrimSpace(ui.RequestInput("select mode")))
				switch mode {
				case "auto", "a":
					ui.Log("Loading COM ports...")

					results, err := getPort()
					if err != nil {
						logrus.WithError(err).Error("getPort error")
						cancel()
						return
					}

					index := -1

					for {
						for _, r := range *results {
							ui.Log(fmt.Sprintf(" %s - %s", r.DeviceID, r.Name))
						}
						port := strings.TrimSpace(ui.RequestInput("select COM port"))

						for i, v := range *results {
							if v.DeviceID == port {
								index = i
								break
							}
						}

						if index != -1 {
							PortName = port
							break
						}

						ui.Log("invalid input")
					}
					break selectmode
				case "custom", "c":
					port := strings.TrimSpace(ui.RequestInput("COM port"))
					if port == "" {
						ui.Log("invalid input")
						continue
					}
					PortName = port
					break selectmode
				default:
					continue
				}
			}

			c := &serial.Config{
				Name: PortName,
				Baud: 115200,
			}
			s, err := serial.OpenPort(c)
			if err != nil {
				ui.Log(fmt.Sprintf("[red]OpenPort error: %v", err))
				continue
			}
			defer s.Close()

			// 接続確立後は入力/プロンプト等のローカルログを抑止し、受信出力のみ表示する。
			ui.SetOutputOnly(true)

			go Writer(ctx, ui, s, cancel)

			ms := make([]byte, 0, 128)
			buf := make([]byte, 128)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					n, err := s.Read(buf)
					if err != nil && err != io.EOF {
						logrus.WithError(err).Error("Read error")
						cancel()
						return
					}
					if n > 0 {
						ms = append(ms, buf[:n]...)
						if buf[n-1] == '\n' {
							ui.LogOutput(string(ms))
							ms = ms[:0]
						}
					}
				}
			}
		}
	}()

	<-ctx.Done()
	ui.Stop()
	logrus.Info("shutting down")
}

func Writer(ctx context.Context, ui *LogUI, s *serial.Port, cancel context.CancelFunc) {
	mode := WriterModeBase64
	ui.Log("writer mode: base64")
	ui.Log("writer commands: /mode, /mode base64|text|hex|int, /help")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			ms := strings.TrimSpace(ui.RequestInput(fmt.Sprintf("message [%s]", mode)))
			if ms == "" {
				continue
			}

			handled, nextMode := HandleWriterCommand(ui, ms, mode, s)
			if handled {
				mode = nextMode
				continue
			}

			payload, err := DecodeWriterInput(mode, ms)
			if err != nil {
				ui.Log(fmt.Sprintf("[red]decode error: %v", err))
				continue
			}

			if _, err := s.Write(payload); err != nil {
				logrus.WithError(err).Error("Write error")
				cancel()
				return
			}

			ui.Log(fmt.Sprintf("[green]TX %d bytes (%s)", len(payload), mode))
		}
	}
}

func HandleWriterCommand(ui *LogUI, input string, mode WriterMode, s *serial.Port) (bool, WriterMode) {
	if !strings.HasPrefix(input, "/") {
		return false, mode
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return true, mode
	}

	switch fields[0] {
	case "/help":
		ui.LogCommand("commands: /mode, /mode base64|text|hex|int")
		ui.LogCommand("text: hello")
		ui.LogCommand("hex: ff 0a 01 or ff0a01 or 0xff,0x0a,0x01")
		ui.LogCommand("int: 255 10 1 (0-255)")
		return true, mode
	case "/mode":
		if len(fields) == 1 {
			ui.LogCommand(fmt.Sprintf("current mode: %s", mode))
			return true, mode
		}

		switch strings.ToLower(fields[1]) {
		case string(WriterModeBase64):
			ui.LogCommand("mode changed: base64")
			return true, WriterModeBase64
		case string(WriterModeText):
			ui.LogCommand("mode changed: text")
			return true, WriterModeText
		case string(WriterModeHex), "binary":
			ui.LogCommand("mode changed: hex")
			return true, WriterModeHex
		case string(WriterModeInt):
			ui.LogCommand("mode changed: int")
			return true, WriterModeInt
		default:
			ui.LogCommand("invalid mode. use: base64 | text | hex | int")
			return true, mode
		}

	case "/apple":
		if err := Badapple(s, ui.LogCommand); err != nil {
			ui.LogCommand(fmt.Sprintf("badapple error: %v", err))
		}
		return true, mode

	default:
		ui.LogCommand("unknown command. use /help")
		return true, mode
	}
}

func DecodeWriterInput(mode WriterMode, input string) ([]byte, error) {
	switch mode {
	case WriterModeBase64:
		dec, err := base64.StdEncoding.DecodeString(input)
		if err == nil {
			return dec, nil
		}
		return base64.RawStdEncoding.DecodeString(input)
	case WriterModeText:
		return []byte(input), nil
	case WriterModeHex:
		return DecodeHexInput(input)
	case WriterModeInt:
		return DecodeIntInput(input)
	default:
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}
}

func DecodeHexInput(input string) ([]byte, error) {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return nil, fmt.Errorf("empty input")
	}

	normalized = strings.ReplaceAll(normalized, ",", " ")
	fields := strings.Fields(normalized)

	if len(fields) > 1 {
		out := make([]byte, 0, len(fields))
		for _, token := range fields {
			original := token
			token = strings.TrimPrefix(strings.ToLower(token), "0x")
			if len(token) == 0 || len(token) > 2 {
				return nil, fmt.Errorf("invalid byte token: %s", original)
			}

			v, err := strconv.ParseUint(token, 16, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid byte token: %s", original)
			}
			out = append(out, byte(v))
		}
		return out, nil
	}

	token := strings.TrimPrefix(strings.ToLower(fields[0]), "0x")
	if len(token)%2 != 0 {
		return nil, fmt.Errorf("hex string length must be even: %d", len(token))
	}

	dec, err := hex.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string")
	}
	return dec, nil
}

func DecodeIntInput(input string) ([]byte, error) {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return nil, fmt.Errorf("empty input")
	}

	normalized = strings.ReplaceAll(normalized, ",", " ")
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	out := make([]byte, 0, len(fields))
	for _, token := range fields {
		v, err := strconv.Atoi(token)
		if err != nil {
			return nil, fmt.Errorf("invalid int token: %s", token)
		}
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("int out of byte range (0-255): %d", v)
		}
		out = append(out, byte(v))
	}
	return out, nil
}
