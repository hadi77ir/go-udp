package raw

// DesiredReceiveBufferSize is the kernel UDP receive buffer size that we'd like to use.
const DesiredReceiveBufferSize = (1 << 20) * 7 // 7 MB

// DesiredSendBufferSize is the kernel UDP send buffer size that we'd like to use.
const DesiredSendBufferSize = (1 << 20) * 7 // 7 MB
