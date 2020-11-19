// Code generated by "genprotocol.exe -ver=455a4b53edb6ef3c7d372deb38edad83bc11b116600c85dcce176a3dfe24b32a -basedir=protocol_me -prefix=me -statstype=int"

package me_const

import "time"

const (
	// MaxBodyLen set to max body len, affect send/recv buffer size
	MaxBodyLen = 0xfffff
	// PacketBufferPoolSize max size of pool packet buffer
	PacketBufferPoolSize = 100

	// ServerAPICallTimeOutDur api call watchdog timer
	ServerAPICallTimeOutDur = time.Second * 2
)
