# Connection-oriented UDP

This package contains code to split a UDP super-connection (that has only one receiving endpoint and no remote endpoint) into 
multiple sub-connections (per remote endpoint).

For example a `net.PacketConn` only has a local address and can receive packets from any remote address, but in this model, 
there would be a **super connection** (a "listener" in TCP terms) that accepts packet connections (upon receiving the first
packet) and therefore it would be more similar to net.Conn, and this abstraction is way more preferred when building
connection-oriented protocols over UDP (e.g. DTLS, QUIC, KCP, WebRTC).

## Credits

This code is mostly borrowed from [`pion/transport`](https://github.com/pion/transport) and is licensed under MIT and
adapted to work with `RawConn`.

Code from `pion` is licensed under MIT license.

```
MIT License

Copyright (c) 2023 The Pion community <https://pion.ly>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```