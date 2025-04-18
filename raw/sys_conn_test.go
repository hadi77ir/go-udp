package raw

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestBasicConn(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	c := NewMockPacketConn(mockCtrl)
	addr := &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 1234}
	c.EXPECT().ReadFrom(gomock.Any()).DoAndReturn(func(b []byte) (int, net.Addr, error) {
		data := []byte("foobar")
		require.Equal(t, MaxPacketBufferSize, len(b))
		return copy(b, data), addr, nil
	})

	conn, err := WrapConn(c, 0)
	require.NoError(t, err)
	buf := make([]byte, MaxPacketBufferSize)
	n, _, _, remoteAddr, err := conn.ReadPacket(buf, []byte{})
	require.NoError(t, err)
	require.Equal(t, []byte("foobar"), buf[:n])
	//require.WithinDuration(t, time.Now(), p.rcvTime, scaleDuration(100*time.Millisecond))
	require.Equal(t, addr, remoteAddr)
}
