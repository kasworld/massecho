// Code generated by "genprotocol.exe -ver=455a4b53edb6ef3c7d372deb38edad83bc11b116600c85dcce176a3dfe24b32a -basedir=protocol_me -prefix=me -statstype=int"

package me_handlenoti

import (
	"fmt"

	"github.com/kasworld/massecho/protocol_me/me_idnoti"
	"github.com/kasworld/massecho/protocol_me/me_obj"
	"github.com/kasworld/massecho/protocol_me/me_packet"
)

// obj base demux fn map

var DemuxNoti2ObjFnMap = [...]func(me interface{}, hd me_packet.Header, body interface{}) error{
	me_idnoti.Invalid: objRecvNotiFn_Invalid, // Invalid not used, make empty packet error

}

// Invalid not used, make empty packet error
func objRecvNotiFn_Invalid(me interface{}, hd me_packet.Header, body interface{}) error {
	robj, ok := body.(*me_obj.NotiInvalid_data)
	if !ok {
		return fmt.Errorf("packet mismatch %v", body)
	}
	return fmt.Errorf("Not implemented %v", robj)
}