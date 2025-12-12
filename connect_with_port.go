package main

/*
#cgo CFLAGS: -std=gnu17
#cgo LDFLAGS: -lsnap7
#include <stdlib.h>

// Minimal snap7 client API surface for setting port before connect.
typedef void* S7Object;
S7Object Cli_Create();
int Cli_SetConnectionType(S7Object Client, unsigned short ConnectionType);
int Cli_SetParam(S7Object Client, int ParamNumber, void *pValue);
int Cli_ConnectTo(S7Object Client, const char* Address, int Rack, int Slot);
int Cli_Disconnect(S7Object Client);
void Cli_Destroy(S7Object *Client);
*/
import "C"

import (
	"fmt"
	"unsafe"

	snap7 "github.com/buboi/snap7-go"
)

const cRemotePortParam = 2

func connectWithPort(opts *connOptions) (*snap7.Snap7Client, error) {
	addr := C.CString(opts.address)
	defer C.free(unsafe.Pointer(addr))

	client := C.Cli_Create()

	if opts.ctype > 0 {
		if rc := C.Cli_SetConnectionType(client, C.ushort(opts.ctype)); rc != 0 {
			C.Cli_Destroy(&client)
			return nil, fmt.Errorf("set connection type: code %d", int(rc))
		}
	}

	if opts.port > 0 {
		port := C.ushort(opts.port)
		if rc := C.Cli_SetParam(client, C.int(cRemotePortParam), unsafe.Pointer(&port)); rc != 0 {
			C.Cli_Destroy(&client)
			return nil, fmt.Errorf("set remote port: code %d", int(rc))
		}
	}

	if rc := C.Cli_ConnectTo(client, addr, C.int(opts.rack), C.int(opts.slot)); rc != 0 {
		C.Cli_Destroy(&client)
		return nil, fmt.Errorf("connect to %s:%d rack=%d slot=%d failed (code %d)", opts.address, opts.port, opts.rack, opts.slot, int(rc))
	}

	s := snap7.Snap7Client{}
	// Snap7Client has a single field inner of type C.S7Object; set it via unsafe.
	*(*C.S7Object)(unsafe.Pointer(&s)) = client
	return &s, nil
}
