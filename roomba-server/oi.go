package main

import (
	"fmt"
	"io"
	"sync"
)

const (
	opStart       = 128
	opSafe        = 131
	opFull        = 132
	opPower       = 133
	opSpot        = 134
	opClean       = 135
	opDriveDirect = 145
	opSeekDock    = 143
	opStop        = 173
)

const (
	defaultSpeed int16 = 200
	turnSpeed    int16 = 150
	curveRatio         = 0.3
)

type Roomba struct {
	port io.ReadWriter
	mu   sync.Mutex
}

func NewRoomba(port io.ReadWriter) *Roomba {
	return &Roomba{port: port}
}

func (r *Roomba) Start() error    { return r.send(opStart) }
func (r *Roomba) Safe() error     { return r.send(opSafe) }
func (r *Roomba) Full() error     { return r.send(opFull) }
func (r *Roomba) Clean() error    { return r.send(opClean) }
func (r *Roomba) Spot() error     { return r.send(opSpot) }
func (r *Roomba) SeekDock() error { return r.send(opSeekDock) }
func (r *Roomba) StopOI() error   { return r.send(opStop) }

func (r *Roomba) DriveDirect(rightVel, leftVel int16) error {
	return r.send(opDriveDirect,
		byte(rightVel>>8), byte(rightVel),
		byte(leftVel>>8), byte(leftVel),
	)
}

func (r *Roomba) DriveStop() error { return r.DriveDirect(0, 0) }
func (r *Roomba) Forward() error   { return r.DriveDirect(defaultSpeed, defaultSpeed) }
func (r *Roomba) Backward() error  { return r.DriveDirect(-defaultSpeed, -defaultSpeed) }
func (r *Roomba) TurnLeft() error  { return r.DriveDirect(turnSpeed, -turnSpeed) }
func (r *Roomba) TurnRight() error { return r.DriveDirect(-turnSpeed, turnSpeed) }

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

func (r *Roomba) send(data ...byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.port.Write(data)
	if err != nil {
		return fmt.Errorf("oi write: %w", err)
	}
	return nil
}

