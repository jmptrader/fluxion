package engine

import (
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/yosisa/fluxion/buffer"
	"github.com/yosisa/fluxion/event"
	"github.com/yosisa/pave/process"
)

type Instance struct {
	eng    *Engine
	enc    event.Encoder
	dec    event.Decoder
	conf   map[string]interface{}
	buf    *buffer.Options
	Router *TagRouter
	rbuf   *RingBuffer
}

func NewInstance(eng *Engine, cmd *process.Command, conf map[string]interface{}, buf *buffer.Options) *Instance {
	i := &Instance{eng: eng, conf: conf, buf: buf, Router: &TagRouter{}, rbuf: NewRingBuffer(1024)}
	cmd.PrepareFunc = func(cmd *exec.Cmd) {
		cmd.Stderr = os.Stderr
		w, _ := cmd.StdinPipe()
		r, _ := cmd.StdoutPipe()
		i.enc = event.NewEncoder(w)
		i.dec = event.NewDecoder(io.TeeReader(r, i.rbuf))
	}
	return i
}

func (i *Instance) eventLoop() {
	for {
		var ev event.Event
		if err := i.dec.Decode(&ev); err != nil {
			b := make([]byte, 1024)
			n, _ := i.rbuf.Read(b)
			log.Fatalf("%v: last read: %x", err, b[:n])
			continue
		}
		i.rbuf.Clear()

		switch ev.Name {
		case "record":
			i.eng.Filter(ev.Record)
		case "next_filter":
			if ins := i.Router.Route(ev.Record.Tag); ins != nil {
				ins.Emit(ev.Record)
			} else {
				i.eng.Emit(ev.Record)
			}
		}
	}
}

func (i *Instance) SetBuffer() error {
	ev := &event.Event{Name: "set_buffer", Buffer: i.buf}
	return i.enc.Encode(ev)
}

func (i *Instance) Configure() error {
	b, err := Encode(i.conf)
	if err != nil {
		return err
	}
	ev := &event.Event{Name: "config", Payload: b}
	return i.enc.Encode(ev)
}

func (i *Instance) Start() error {
	go i.eventLoop()
	ev := &event.Event{Name: "start"}
	return i.enc.Encode(ev)
}

func (i *Instance) Emit(record *event.Record) error {
	ev := &event.Event{Name: "record", Record: record}
	return i.enc.Encode(ev)
}
