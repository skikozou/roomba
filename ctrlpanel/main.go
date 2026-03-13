package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tarm/serial"
)

// devNull は読み書き両方を捨てるダミーReadWriter（デバッグ用）
type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }
func (devNull) Read(p []byte) (int, error)  { return 0, io.EOF }

func openSerial(portName string) (io.ReadWriteCloser, error) {
	c := &serial.Config{
		Name:        portName,
		Baud:        115200,
		ReadTimeout: 500, // ms
	}
	return serial.OpenPort(c)
}

func main() {
	cfg, err := parseLaunchConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprintf(os.Stderr, "usage: %s [port] [--sound <file>] [--ui-only]\n", os.Args[0])
		os.Exit(2)
	}

	if cfg.uiOnly {
		portLabel := cfg.port
		if strings.TrimSpace(portLabel) == "" {
			portLabel = "ui-only"
		}

		fake := newFakeRoombaPort()
		roomba := NewRoomba(fake)
		roomba.Start()
		roomba.Safe()

		m := newPanelModel(portLabel, roomba, cfg.soundFile)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		roomba.DriveStop()
		return
	}

	if cfg.port != "" {
		port := cfg.port
		conn, err := openSerial(port)
		if err != nil {
			// デバッグモード: ダミーセンサーデータを返すReadWriter
			fake := newFakeRoombaPort()
			m := newPanelModel(port, NewRoomba(fake), cfg.soundFile)
			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
		defer conn.Close()

		roomba := NewRoomba(conn)
		roomba.Start()
		roomba.Safe()

		m := newPanelModel(port, roomba, cfg.soundFile)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		roomba.DriveStop()
		return
	}

	ports := listACMPorts()
	if len(ports) == 0 {
		fmt.Fprintln(os.Stderr, "no /dev/ttyACM* devices found")
		os.Exit(1)
	}

	m := newSelectModel(ports, cfg.soundFile)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type launchConfig struct {
	port      string
	soundFile string
	uiOnly    bool
}

func parseLaunchConfig(args []string) (launchConfig, error) {
	var cfg launchConfig

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--ui-only", "--fake":
			cfg.uiOnly = true
		case "--sound", "-sound", "-s":
			if i+1 >= len(args) {
				return cfg, fmt.Errorf("%s requires a file path", arg)
			}
			i++
			cfg.soundFile = strings.TrimSpace(args[i])
			if cfg.soundFile == "" {
				return cfg, fmt.Errorf("%s requires a non-empty file path", arg)
			}
		default:
			if strings.HasPrefix(arg, "-") {
				return cfg, fmt.Errorf("unknown option: %s", arg)
			}
			if cfg.port != "" {
				return cfg, fmt.Errorf("multiple ports specified: %s %s", cfg.port, arg)
			}
			cfg.port = arg
		}
	}

	return cfg, nil
}

// fakeRoombaPort はデバッグ用のダミーポート
// QueryList コマンドを受けるとダミーセンサーデータを返す
type fakeRoombaPort struct {
	readBuf *bytes.Buffer
}

func newFakeRoombaPort() *fakeRoombaPort {
	return &fakeRoombaPort{readBuf: &bytes.Buffer{}}
}

func (f *fakeRoombaPort) Write(p []byte) (int, error) {
	// QueryList (149) を検出したらダミーレスポンスを用意
	if len(p) > 0 && p[0] == opQueryList {
		// OIMode=2(Safe), Charge=1500mAh, Capacity=2600mAh, Voltage=14400mV, Temp=25°C, Charging=0, Bumps=0
		resp := []byte{
			2,          // OI Mode: Safe
			0x05, 0xDC, // Battery Charge: 1500
			0x0A, 0x28, // Battery Capacity: 2600
			0x38, 0x40, // Voltage: 14400 mV
			25, // Temperature: 25°C
			0,  // Charging State: Not charging
			0,  // Bumps: none
		}
		f.readBuf.Write(resp)
	}
	return len(p), nil
}

func (f *fakeRoombaPort) Read(p []byte) (int, error) {
	if f.readBuf.Len() == 0 {
		return 0, io.EOF
	}
	return f.readBuf.Read(p)
}
