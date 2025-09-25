//go:build windows

package raw

import (
	"net"
	"os"
)

// Windows-specific constants
const (
	msgTypeIPTOS   = 1
	ipv4PKTINFO    = 19
	ECNIPv4DataLen = 4
	batchSize      = 32 // Windows can handle larger batches than Darwin
)

// MaxPacketBufferSize maximum packet size of any QUIC packet, based on
// ethernet's max size (1500) minus the IP header (20 for IPv4, 40 for IPv6)
// minus the UDP header (8).
// https://en.wikipedia.org/wiki/Ethernet_frame#Ethernet_II
const MaxPacketBufferSize = 1452

// ParseIPv4PktInfo parses IPv4 packet info on Windows
func ParseIPv4PktInfo(oob []byte) (ip net.IP, ifIndex uint32) {
	// Windows doesn't support packet info in the same way as Unix systems
	return nil, 0
}

// IsGSOEnabled checks if Generic Segmentation Offload is enabled on Windows
func IsGSOEnabled(conn net.PacketConn) bool {
	// Windows doesn't support GSO in the same way as Linux
	return false
}

// IsECNEnabled checks if ECN is enabled on Windows
func IsECNEnabled(conn net.PacketConn) bool {
	// Check if ECN is disabled via environment variable
	if IsECNDisabledUsingEnv() {
		return false
	}
	// Windows has limited ECN support
	return false
}

// IsECNDisabledUsingEnv checks if ECN is disabled using environment variable
func IsECNDisabledUsingEnv() bool {
	return os.Getenv("QUIC_GO_DISABLE_ECN") == "true"
}
