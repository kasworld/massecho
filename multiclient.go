// Copyright 2015,2016,2017,2018,2019,2020 SeukWon Kang (kasworld@gmail.com)
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/kasworld/argdefault"
	"github.com/kasworld/massecho/protocol_me/me_conntcp"
	"github.com/kasworld/massecho/protocol_me/me_connwsgorilla"
	"github.com/kasworld/massecho/protocol_me/me_idcmd"
	"github.com/kasworld/massecho/protocol_me/me_msgp"
	"github.com/kasworld/massecho/protocol_me/me_obj"
	"github.com/kasworld/massecho/protocol_me/me_packet"
	"github.com/kasworld/massecho/protocol_me/me_pid2rspfn"
	"github.com/kasworld/multirun"
	"github.com/kasworld/prettystring"
	"github.com/kasworld/rangestat"
)

// service const
const (
	// for client
	readTimeoutSec  = 6 * time.Second
	writeTimeoutSec = 3 * time.Second
)

var marshalBodyFn func(body interface{}, oldBuffToAppend []byte) ([]byte, byte, error)
var unmarshalPacketFn func(h me_packet.Header, bodyData []byte) (interface{}, error)

type MultiClientConfig struct {
	ConnectToServer  string `default:"localhost:8080" argname:""`
	NetType          string `default:"ws" argname:""`
	PlayerNameBase   string `default:"MC_" argname:""`
	Concurrent       int    `default:"50000" argname:""`
	AccountPool      int    `default:"0" argname:""`
	AccountOverlap   int    `default:"0" argname:""`
	LimitStartCount  int    `default:"0" argname:""`
	LimitEndCount    int    `default:"0" argname:""`
	PacketIntervalMS int    `default:"1000" argname:""` // milisecond
}

func main() {
	ads := argdefault.New(&MultiClientConfig{})
	ads.RegisterFlag()
	flag.Parse()
	config := &MultiClientConfig{}
	ads.SetDefaultToNonZeroField(config)
	ads.ApplyFlagTo(config)
	fmt.Println(prettystring.PrettyString(config, 4))

	marshalBodyFn = me_msgp.MarshalBodyFn
	unmarshalPacketFn = me_msgp.UnmarshalPacket

	chErr := make(chan error)
	go multirun.Run(
		context.Background(),
		config.Concurrent,
		config.AccountPool,
		config.AccountOverlap,
		config.LimitStartCount,
		config.LimitEndCount,
		func(config interface{}) multirun.ClientI {
			return NewApp(config.(AppArg))
		},
		func(i int) interface{} {
			return AppArg{
				ConnectToServer:  config.ConnectToServer,
				NetType:          config.NetType,
				Nickname:         fmt.Sprintf("%v%v", config.PlayerNameBase, i),
				SessionUUID:      "",
				Auth:             "",
				PacketIntervalMS: config.PacketIntervalMS,
			}
		},
		chErr,
		rangestat.New("", 0, config.Concurrent),
	)
	for err := range chErr {
		fmt.Printf("%v\n", err)
	}
}

type AppArg struct {
	ConnectToServer  string
	NetType          string
	Nickname         string
	SessionUUID      string
	Auth             string
	PacketIntervalMS int
}

type App struct {
	config            AppArg
	c2scWS            *me_connwsgorilla.Connection
	c2scTCP           *me_conntcp.Connection
	EnqueueSendPacket func(pk me_packet.Packet) error
	runResult         error

	sendRecvStop func()
	pid2recv     *me_pid2rspfn.PID2RspFn
}

func NewApp(config AppArg) *App {
	app := &App{
		config:   config,
		pid2recv: me_pid2rspfn.New(),
	}
	return app
}

func (app *App) String() string {
	return fmt.Sprintf("App[%v %v]", app.config.Nickname, app.config.SessionUUID)
}

func (app *App) GetArg() interface{} {
	return app.config
}

func (app *App) GetRunResult() error {
	return app.runResult
}

func (app *App) Run(mainctx context.Context) {
	ctx, stopFn := context.WithCancel(mainctx)
	app.sendRecvStop = stopFn
	defer app.sendRecvStop()

	switch app.config.NetType {
	default:
		fmt.Printf("unsupported nettype %v\n", app.config.NetType)
		return
	case "tcp":
		app.connectTCP(ctx)
	case "ws":
		app.connectWS(ctx)
	}

	if app.config.PacketIntervalMS == 0 {
		go app.reqEcho()
		for {
			select {
			case <-ctx.Done():
				return
			}
		}
	} else {
		timerPingTk := time.NewTicker(time.Millisecond * time.Duration(app.config.PacketIntervalMS))
		defer timerPingTk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timerPingTk.C:
				go app.reqEcho()

			}
		}
	}

}

func (app *App) connectWS(ctx context.Context) {
	app.c2scWS = me_connwsgorilla.New(
		readTimeoutSec, writeTimeoutSec,
		marshalBodyFn,
		app.handleRecvPacket,
		app.handleSentPacket,
	)
	if err := app.c2scWS.ConnectTo(app.config.ConnectToServer); err != nil {
		app.runResult = err
		fmt.Printf("%v\n", err)
		app.sendRecvStop()
		return
	}
	app.EnqueueSendPacket = app.c2scWS.EnqueueSendPacket
	go func(ctx context.Context) {
		app.runResult = app.c2scWS.Run(ctx)
	}(ctx)
}

func (app *App) connectTCP(ctx context.Context) {
	app.c2scTCP = me_conntcp.New(
		readTimeoutSec, writeTimeoutSec,
		marshalBodyFn,
		app.handleRecvPacket,
		app.handleSentPacket,
	)
	if err := app.c2scTCP.ConnectTo(app.config.ConnectToServer); err != nil {
		app.runResult = err
		fmt.Printf("%v\n", err)
		app.sendRecvStop()
		return
	}
	app.EnqueueSendPacket = app.c2scTCP.EnqueueSendPacket
	go func(ctx context.Context) {
		app.runResult = app.c2scTCP.Run(ctx)
	}(ctx)
}

func (app *App) reqEcho() error {
	msg := ""
	// msg := fmt.Sprintf("hello world from %v", app.config.Nickname)
	return app.ReqWithRspFn(
		me_idcmd.Echo,
		&me_obj.ReqEcho_data{Msg: msg},
		func(hd me_packet.Header, rsp interface{}) error {
			go app.reqEcho()
			return nil
		},
	)
}

func (app *App) handleSentPacket(header me_packet.Header) error {
	return nil
}

func (app *App) handleRecvPacket(header me_packet.Header, body []byte) error {
	robj, err := unmarshalPacketFn(header, body)
	if err != nil {
		return err
	}

	switch header.FlowType {
	default:
		return fmt.Errorf("Invalid packet type %v %v", header, body)
	case me_packet.Notification:
		//process noti here
		// robj, err := unmarshalPacketFn(header, body)

	case me_packet.Response:
		// process response
		if err := app.pid2recv.HandleRsp(header, robj); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) ReqWithRspFn(cmd me_idcmd.CommandID, body interface{},
	fn me_pid2rspfn.HandleRspFn) error {

	pid := app.pid2recv.NewPID(fn)
	spk := me_packet.Packet{
		Header: me_packet.Header{
			Cmd:      uint16(cmd),
			ID:       pid,
			FlowType: me_packet.Request,
		},
		Body: body,
	}

	if err := app.EnqueueSendPacket(spk); err != nil {
		fmt.Printf("End %v %v %v\n", app, spk, err)
		app.sendRecvStop()
		return fmt.Errorf("Send fail %v %v", app, err)
	}
	return nil
}
