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
	"github.com/kasworld/massecho/protocol_me/me_authorize"
	"github.com/kasworld/massecho/protocol_me/me_connbytemanager"
	"github.com/kasworld/massecho/protocol_me/me_error"
	"github.com/kasworld/massecho/protocol_me/me_gob"
	"github.com/kasworld/massecho/protocol_me/me_idcmd"
	"github.com/kasworld/massecho/protocol_me/me_obj"
	"github.com/kasworld/massecho/protocol_me/me_packet"
	"github.com/kasworld/massecho/protocol_me/me_serveconnbyte"
	"github.com/kasworld/massecho/protocol_me/me_statapierror"
	"github.com/kasworld/massecho/protocol_me/me_statnoti"
	"github.com/kasworld/massecho/protocol_me/me_statserveapi"
	"github.com/kasworld/uuidstr"
)

// service const
const (
	sendBufferSize  = 10
	readTimeoutSec  = 6 * time.Second
	writeTimeoutSec = 3 * time.Second
)

func main() {
	httpport := flag.String("httpport", ":8080", "Serve httpport")
	httpfolder := flag.String("httpdir", "www", "Serve http Dir")
	tcpport := flag.String("tcpport", ":8081", "Serve tcpport")
	flag.Parse()

	svr := NewServer()
	svr.Run(*tcpport, *httpport, *httpfolder)
}

type Server struct {
	sendRecvStop func()

	connManager *me_connbytemanager.Manager

	SendStat *actpersec.ActPerSec `prettystring:"simple"`
	RecvStat *actpersec.ActPerSec `prettystring:"simple"`

	apiStat                *me_statserveapi.StatServeAPI
	notiStat               *me_statnoti.StatNotification
	errStat                *me_statapierror.StatAPIError
	marshalBodyFn          func(body interface{}, oldBuffToAppend []byte) ([]byte, byte, error)
	unmarshalPacketFn      func(h me_packet.Header, bodyData []byte) (interface{}, error)
	DemuxReq2BytesAPIFnMap [me_idcmd.CommandID_Count]func(
		me interface{}, hd me_packet.Header, rbody []byte) (
		me_packet.Header, interface{}, error)
}

func NewServer() *Server {
	svr := &Server{
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

	svr.marshalBodyFn = me_gob.MarshalBodyFn
	svr.unmarshalPacketFn = me_gob.UnmarshalPacket
	svr.DemuxReq2BytesAPIFnMap = [...]func(
		me interface{}, hd me_packet.Header, rbody []byte) (
		me_packet.Header, interface{}, error){
		me_idcmd.Invalid: svr.bytesAPIFn_ReqInvalid,
		me_idcmd.Echo:    svr.bytesAPIFn_ReqEcho,
	}
	return svr
}

func (svr *Server) Run(tcpport string, httpport string, httpfolder string) {
	ctx, stopFn := context.WithCancel(context.Background())
	svr.sendRecvStop = stopFn
	defer svr.sendRecvStop()

	go svr.serveTCP(ctx, tcpport)
	go svr.serveHTTP(ctx, httpport, httpfolder)

	timerInfoTk := time.NewTicker(1 * time.Second)
	defer timerInfoTk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timerInfoTk.C:
			svr.SendStat.UpdateLap()
			svr.RecvStat.UpdateLap()
			fmt.Printf("Send:%v Recv:%v\n", svr.SendStat, svr.RecvStat)
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

	connID := uuidstr.New()
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
		readTimeoutSec, writeTimeoutSec, svr.marshalBodyFn)

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

	connID := uuidstr.New()
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
	c2sc.StartServeTCP(ctx, conn,
		readTimeoutSec, writeTimeoutSec, svr.marshalBodyFn)

	// connection cleanup here
	conn.Close()

	// del from conn manager
	svr.connManager.Del(connID)
}

///////////////////////////////////////////////////////////////

func (svr *Server) bytesAPIFn_ReqInvalid(
	me interface{}, hd me_packet.Header, rbody []byte) (
	me_packet.Header, interface{}, error) {
	// robj, err := gUnmarshalPacket(hd, rbody)
	// if err != nil {
	// 	return hd, nil, fmt.Errorf("Packet type miss match %v", rbody)
	// }
	// recvBody, ok := robj.(*me_obj.ReqInvalid_data)
	// if !ok {
	// 	return hd, nil, fmt.Errorf("Packet type miss match %v", robj)
	// }
	// _ = recvBody

	hd.ErrorCode = me_error.None
	sendBody := &me_obj.RspInvalid_data{}
	return hd, sendBody, nil
}

func (svr *Server) bytesAPIFn_ReqEcho(
	me interface{}, hd me_packet.Header, rbody []byte) (
	me_packet.Header, interface{}, error) {
	robj, err := svr.unmarshalPacketFn(hd, rbody)
	if err != nil {
		return hd, nil, fmt.Errorf("Packet type miss match %v", rbody)
	}
	recvBody, ok := robj.(*me_obj.ReqEcho_data)
	if !ok {
		return hd, nil, fmt.Errorf("Packet type miss match %v", robj)
	}
	_ = recvBody

	hd.ErrorCode = me_error.None
	sendBody := &me_obj.RspEcho_data{recvBody.Msg}
	return hd, sendBody, nil
}
