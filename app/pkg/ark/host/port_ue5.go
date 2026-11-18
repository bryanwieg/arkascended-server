package host

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"time"
)

// ProbeUE5UDP sends a UE5 stateless connect handshake initiation packet and
// returns true if the target responds with a server handshake reply.
//
// ARK: Survival Ascended uses the Unreal Engine 5 NetDriver stateless connect
// protocol on its game port. The initial client packet is 48 bytes with the
// magic header 0xa8 0x01 0x02; a valid server responds with 0xa8 0x01 0x82
// (the 0x80 bit in byte 2 indicates the server direction).
//
// This probe also succeeds against the CLI's own --listen echo service,
// which reflects any payload back — making it the single reliable UDP probe
// for both a running ARK server and the synthetic listener.
func ProbeUE5UDP(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout(ProtocolUDP, fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if _, err := conn.Write(buildUE5HandshakePacket()); err != nil {
		return false
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	return err == nil && n > 0
}

// buildUE5HandshakePacket constructs the 48-byte UE5 stateless connect
// initiation packet observed during ARK client connections.
func buildUE5HandshakePacket() []byte {
	pkt := make([]byte, 48)
	pkt[0] = 0xa8
	pkt[1] = 0x01
	pkt[2] = 0x02 // client-to-server direction
	pkt[3] = 0x80
	pkt[4] = 0x00
	binary.LittleEndian.PutUint32(pkt[5:9], rand.Uint32())
	// bytes 9-38: zero (cookie area)
	ts := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	binary.LittleEndian.PutUint64(pkt[39:47], ts)
	pkt[47] = 0x01
	return pkt
}
