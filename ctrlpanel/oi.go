package main

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// OI Opcodes
const (
	opStart       = 128
	opSafe        = 131
	opFull        = 132
	opPower       = 133
	opSpot        = 134
	opClean       = 135
	opMotors      = 138
	opSong        = 140
	opPlay        = 141
	opSensors     = 142
	opSeekDock    = 143
	opDriveDirect = 145
	opQueryList   = 149
	opStop        = 173
)

// Sensor Packet IDs
const (
	pktBumpsWheelDrops = 7
	pktChargingState   = 21
	pktVoltage         = 22
	pktTemperature     = 24
	pktBatteryCharge   = 25
	pktBatteryCapacity = 26
	pktOIMode          = 35
)

// SensorData はポーリングで取得するセンサー値
type SensorData struct {
	OIMode          byte   // 0=Off, 1=Passive, 2=Safe, 3=Full
	BatteryCharge   uint16 // mAh
	BatteryCapacity uint16 // mAh
	Voltage         uint16 // mV
	Temperature     int8   // °C
	ChargingState   byte   // 0-5
	BumpsWheelDrops byte   // bitmask
}

func (s SensorData) BatteryPercent() int {
	if s.BatteryCapacity == 0 {
		return 0
	}
	return int(s.BatteryCharge) * 100 / int(s.BatteryCapacity)
}

func (s SensorData) ModeName() string {
	switch s.OIMode {
	case 0:
		return "Off"
	case 1:
		return "Passive"
	case 2:
		return "Safe"
	case 3:
		return "Full"
	default:
		return "Unknown"
	}
}

func (s SensorData) ChargingStateName() string {
	switch s.ChargingState {
	case 0:
		return "Not charging"
	case 1:
		return "Reconditioning"
	case 2:
		return "Full Charging"
	case 3:
		return "Trickle"
	case 4:
		return "Waiting"
	case 5:
		return "Fault"
	default:
		return "Unknown"
	}
}

// Roomba はシリアルポートへの読み書きインターフェース
type Roomba struct {
	port io.ReadWriter
	mu   sync.Mutex
}

func NewRoomba(port io.ReadWriter) *Roomba {
	return &Roomba{port: port}
}

// ── Mode commands ──

func (r *Roomba) Start() error {
	return r.send(opStart)
}

func (r *Roomba) Safe() error {
	return r.send(opSafe)
}

func (r *Roomba) Full() error {
	return r.send(opFull)
}

func (r *Roomba) PowerOff() error {
	return r.send(opPower)
}

func (r *Roomba) StopOI() error {
	return r.send(opStop)
}

// ── Cleaning commands ──

func (r *Roomba) Clean() error {
	return r.send(opClean)
}

func (r *Roomba) Spot() error {
	return r.send(opSpot)
}

func (r *Roomba) SeekDock() error {
	return r.send(opSeekDock)
}

// SetMotors はブラシ/バキュームモーターのON/OFFを設定する
// bit0=Side Brush, bit1=Vacuum, bit2=Main Brush
func (r *Roomba) SetMotors(mainBrush, vacuum, sideBrush bool) error {
	var bits byte
	if sideBrush {
		bits |= 1 << 0
	}
	if vacuum {
		bits |= 1 << 1
	}
	if mainBrush {
		bits |= 1 << 2
	}
	return r.send(opMotors, bits)
}

// DefineSong は song slot(0-3)にノート列を定義する。
// notes は [midi, len] のペア列で、最大16ノート(=32byte)。
func (r *Roomba) DefineSong(slot byte, notes []byte) error {
	if len(notes) == 0 {
		return fmt.Errorf("empty song data")
	}
	if len(notes)%2 != 0 {
		return fmt.Errorf("song data must be midi/len pairs: got %d bytes", len(notes))
	}
	if len(notes) > 32 {
		return fmt.Errorf("song data too long: %d bytes (max 32)", len(notes))
	}

	payload := make([]byte, 0, 3+len(notes))
	payload = append(payload, opSong, slot, byte(len(notes)/2))
	payload = append(payload, notes...)
	return r.send(payload...)
}

func (r *Roomba) PlaySong(slot byte) error {
	return r.send(opPlay, slot)
}

// ── Drive commands ──

// DriveDirect は左右の車輪速度を独立指定する (mm/s, -500〜500)
func (r *Roomba) DriveDirect(rightVel, leftVel int16) error {
	return r.send(opDriveDirect,
		byte(rightVel>>8), byte(rightVel),
		byte(leftVel>>8), byte(leftVel),
	)
}

// DriveStop は両輪を停止する
func (r *Roomba) DriveStop() error {
	return r.DriveDirect(0, 0)
}

// ── 高レベルドライブ ──

const (
	defaultSpeed int16 = 200 // mm/s
	turnSpeed    int16 = 150 // mm/s (その場旋回)
	curveRatio         = 0.3 // 曲がる側の速度比 (1.0=直進, 0.0=片輪停止)
)

func (r *Roomba) Forward() error {
	return r.DriveDirect(defaultSpeed, defaultSpeed)
}

func (r *Roomba) Backward() error {
	return r.DriveDirect(-defaultSpeed, -defaultSpeed)
}

func (r *Roomba) ForwardLeft() error {
	slow := int16(float64(defaultSpeed) * curveRatio)
	return r.DriveDirect(defaultSpeed, slow)
}

func (r *Roomba) ForwardRight() error {
	slow := int16(float64(defaultSpeed) * curveRatio)
	return r.DriveDirect(slow, defaultSpeed)
}

func (r *Roomba) BackwardLeft() error {
	slow := int16(float64(defaultSpeed) * curveRatio)
	return r.DriveDirect(-defaultSpeed, -slow)
}

func (r *Roomba) BackwardRight() error {
	slow := int16(float64(defaultSpeed) * curveRatio)
	return r.DriveDirect(-slow, -defaultSpeed)
}

func (r *Roomba) TurnLeft() error {
	return r.DriveDirect(turnSpeed, -turnSpeed)
}

func (r *Roomba) TurnRight() error {
	return r.DriveDirect(-turnSpeed, turnSpeed)
}

// ── Sensors ──

// queryPackets はポーリングで取得するパケットIDリスト
var queryPackets = []byte{
	pktOIMode,
	pktBatteryCharge,
	pktBatteryCapacity,
	pktVoltage,
	pktTemperature,
	pktChargingState,
	pktBumpsWheelDrops,
}

// QuerySensors はセンサーデータを1回ポーリングする
// 応答: OIMode(1) + Charge(2) + Capacity(2) + Voltage(2) + Temp(1) + ChargingState(1) + Bumps(1) = 10 bytes
const sensorResponseLen = 10

func (r *Roomba) QuerySensors() (SensorData, error) {
	var lastErr error
	for attempt := 1; attempt <= sensorQueryRetries; attempt++ {
		sd, err := r.querySensorsOnce()
		if err == nil {
			return sd, nil
		}
		lastErr = err
		if attempt < sensorQueryRetries {
			time.Sleep(sensorRetryDelay)
		}
	}

	var zero SensorData
	return zero, lastErr
}

const (
	sensorQueryRetries = 3
	sensorRetryDelay   = 80 * time.Millisecond
)

func (r *Roomba) querySensorsOnce() (SensorData, error) {
	var sd SensorData

	cmd := make([]byte, 0, 2+len(queryPackets))
	cmd = append(cmd, opQueryList, byte(len(queryPackets)))
	cmd = append(cmd, queryPackets...)

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.sendLocked(cmd...); err != nil {
		return sd, err
	}

	buf := make([]byte, sensorResponseLen)
	n, err := io.ReadFull(r.port, buf)
	if err != nil {
		return sd, fmt.Errorf("oi read sensors (%d/%d bytes): %w", n, sensorResponseLen, err)
	}

	sd = decodeSensorData(buf)
	if !sd.isPlausible() {
		return sd, fmt.Errorf("oi read sensors: implausible response: %v", buf)
	}

	return sd, nil
}

func decodeSensorData(buf []byte) SensorData {
	var sd SensorData
	sd.OIMode = buf[0]
	sd.BatteryCharge = uint16(buf[1])<<8 | uint16(buf[2])
	sd.BatteryCapacity = uint16(buf[3])<<8 | uint16(buf[4])
	sd.Voltage = uint16(buf[5])<<8 | uint16(buf[6])
	sd.Temperature = int8(buf[7])
	sd.ChargingState = buf[8]
	sd.BumpsWheelDrops = buf[9]
	return sd
}

func (s SensorData) isPlausible() bool {
	if s.OIMode > 3 {
		return false
	}
	if s.ChargingState > 5 {
		return false
	}
	if s.Voltage < 5000 || s.Voltage > 25000 {
		return false
	}
	if s.Temperature < -40 || s.Temperature > 100 {
		return false
	}
	if s.BatteryCapacity > 0 && s.BatteryCharge > s.BatteryCapacity*2 {
		return false
	}
	return true
}

// ── internal ──

func (r *Roomba) send(data ...byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sendLocked(data...)
}

func (r *Roomba) sendLocked(data ...byte) error {
	_, err := r.port.Write(data)
	if err != nil {
		return fmt.Errorf("oi write: %w", err)
	}
	return nil
}
