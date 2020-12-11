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
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kasworld/actpersec"
	"github.com/kasworld/argdefault"
	"github.com/kasworld/massecho/lib/idu64str"
	"github.com/kasworld/massecho/protocol_me/me_authorize"
	"github.com/kasworld/massecho/protocol_me/me_connbytemanager"
	"github.com/kasworld/massecho/protocol_me/me_error"
	"github.com/kasworld/massecho/protocol_me/me_idcmd"
	"github.com/kasworld/massecho/protocol_me/me_msgp"
	"github.com/kasworld/massecho/protocol_me/me_obj"
	"github.com/kasworld/massecho/protocol_me/me_packet"
	"github.com/kasworld/massecho/protocol_me/me_serveconnbyte"
	"github.com/kasworld/massecho/protocol_me/me_statapierror"
	"github.com/kasworld/massecho/protocol_me/me_statnoti"
	"github.com/kasworld/massecho/protocol_me/me_statserveapi"
	"github.com/kasworld/prettystring"
)

// service const
const (
	sendBufferSize  = 10
	readTimeoutSec  = 6 * time.Second
	writeTimeoutSec = 3 * time.Second
)

type ServerConfig struct {
	TcpPort    string `default:":8081" argname:""`
	HttpPort   string `default:":8080" argname:""`
	HttpFolder string `default:"www" argname:""`
}

func main() {
	ads := argdefault.New(&ServerConfig{})
	ads.RegisterFlag()
	flag.Parse()
	config := &ServerConfig{}
	ads.SetDefaultToNonZeroField(config)
	ads.ApplyFlagTo(config)
	fmt.Println(prettystring.PrettyString(config, 4))

	svr := NewServer(config)
	svr.Run()
}

var marshalBodyFn = me_msgp.MarshalBodyFn
var unmarshalPacketFn = me_msgp.UnmarshalPacket

type Server struct {
	config       *ServerConfig
	sendRecvStop func()

	connManager *me_connbytemanager.Manager

	SendStat *actpersec.ActPerSec `prettystring:"simple"`
	RecvStat *actpersec.ActPerSec `prettystring:"simple"`

	apiStat                *me_statserveapi.StatServeAPI
	notiStat               *me_statnoti.StatNotification
	errStat                *me_statapierror.StatAPIError
	DemuxReq2BytesAPIFnMap [me_idcmd.CommandID_Count]func(
		me interface{}, hd me_packet.Header, rbody []byte) (
		me_packet.Header, interface{}, error)
}

func NewServer(config *ServerConfig) *Server {
	svr := &Server{
		config:      config,
		connManager: me_connbytemanager.New(),
		SendStat:    actpersec.New(),
		RecvStat:    actpersec.New(),
		apiStat:     me_statserveapi.New(),
		notiStat:    me_statnoti.New(),
		errStat:     me_statapierror.New(),
	}
	svr.sendRecvStop = func() {
		fmt.Printf("Too early sendRecvStop call\n")
	}

	svr.DemuxReq2BytesAPIFnMap = [...]func(
		me interface{}, hd me_packet.Header, rbody []byte) (
		me_packet.Header, interface{}, error){
		me_idcmd.Invalid: svr.bytesAPIFn_ReqInvalid,
		me_idcmd.Echo:    svr.bytesAPIFn_ReqEcho,
	}
	return svr
}

func (svr *Server) Run() {
	ctx, stopFn := context.WithCancel(context.Background())
	svr.sendRecvStop = stopFn
	defer svr.sendRecvStop()

	go svr.serveTCP(ctx, svr.config.TcpPort)
	go svr.serveHTTP(ctx, svr.config.HttpPort, svr.config.HttpFolder)

	timerInfoTk := time.NewTicker(1 * time.Second)
	defer timerInfoTk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timerInfoTk.C:
			svr.SendStat.UpdateLap()
			svr.RecvStat.UpdateLap()
			fmt.Printf("Connection:%v Send:%v Recv:%v\n",
				svr.connManager.Len(),
				svr.SendStat, svr.RecvStat)
		}
	}
}

func (svr *Server) serveHTTP(ctx context.Context, port string, folder string) {
	fmt.Printf("http server dir=%v port=%v , http://localhost%v/\n",
		folder, port, port)
	webMux := http.NewServeMux()
	webMux.Handle("/",
		http.FileServer(http.Dir(folder)),
	)
	webMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		svr.serveWebSocketClient(ctx, w, r)
	})
	if err := http.ListenAndServe(port, webMux); err != nil {
		fmt.Println(err.Error())
	}
}

func CheckOrigin(r *http.Request) bool {
	return true
}

func (svr *Server) serveWebSocketClient(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: CheckOrigin,
	}
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("upgrade %v\n", err)
		return
	}

	connID := idu64str.G_Maker.New()
	c2sc := me_serveconnbyte.NewWithStats(
		connID,
		sendBufferSize,
		me_authorize.NewAllSet(),
		svr.SendStat, svr.RecvStat,
		svr.apiStat,
		svr.notiStat,
		svr.errStat,
		svr.DemuxReq2BytesAPIFnMap)

	// add to conn manager
	svr.connManager.Add(connID, c2sc)

	// start client service
	c2sc.StartServeWS(ctx, wsConn,
		readTimeoutSec, writeTimeoutSec, marshalBodyFn)

	// connection cleanup here
	wsConn.Close()

	// del from conn manager
	svr.connManager.Del(connID)
}

func (svr *Server) serveTCP(ctx context.Context, port string) {
	fmt.Printf("tcp server port=%v\n", port)
	tcpaddr, err := net.ResolveTCPAddr("tcp", port)
	if err != nil {
		fmt.Printf("error %v\n", err)
		return
	}
	listener, err := net.ListenTCP("tcp", tcpaddr)
	if err != nil {
		fmt.Printf("error %v\n", err)
		return
	}
	defer listener.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			listener.SetDeadline(time.Now().Add(time.Duration(1 * time.Second)))
			conn, err := listener.AcceptTCP()
			if err != nil {
				operr, ok := err.(*net.OpError)
				if ok && operr.Timeout() {
					continue
				}
				fmt.Printf("error %#v\n", err)
			} else {
				go svr.serveTCPClient(ctx, conn)
			}
		}
	}
}

func (svr *Server) serveTCPClient(ctx context.Context, conn *net.TCPConn) {

	connID := idu64str.G_Maker.New()

	c2sc := me_serveconnbyte.New(
		connID,
		sendBufferSize,
		me_authorize.NewAllSet(),
		svr.SendStat, svr.RecvStat,
		svr.DemuxReq2BytesAPIFnMap)

	// add to conn manager
	svr.connManager.Add(connID, c2sc)

	// start client service
	c2sc.StartServeTCP(ctx, conn,
		readTimeoutSec, writeTimeoutSec, marshalBodyFn)

	// connection cleanup here
	c2sc.Disconnect()
	conn.Close()

	// del from conn manager
	svr.connManager.Del(connID)
}

///////////////////////////////////////////////////////////////

func (svr *Server) bytesAPIFn_ReqInvalid(
	me interface{}, hd me_packet.Header, rbody []byte) (
	me_packet.Header, interface{}, error) {
	return hd, nil, fmt.Errorf("invalid packet")
}

func (svr *Server) bytesAPIFn_ReqEcho(
	me interface{}, hd me_packet.Header, rbody []byte) (
	me_packet.Header, interface{}, error) {

	// msgp only
	var recvBody me_obj.ReqEcho_data
	if _, err := recvBody.UnmarshalMsg(rbody); err != nil {
		return hd, nil, err
	}

	hd.ErrorCode = me_error.None
	sendBody := &me_obj.RspEcho_data{recvBody.Msg}
	return hd, sendBody, nil
}
