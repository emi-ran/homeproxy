package proxy

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func TestSharedAddressPolicy(t *testing.T) {
	for _, host := range []string{"100.64.0.0", "100.100.100.200", "100.127.255.255", "::ffff:100.100.100.200"} {
		if _, err := safeTarget(context.Background(), net.JoinHostPort(host, "80"), false); err == nil {
			t.Errorf("shared address accepted: %s", host)
		}
	}
	for _, host := range []string{"100.63.255.255", "100.128.0.0"} {
		if _, err := safeTarget(context.Background(), net.JoinHostPort(host, "80"), false); err != nil {
			t.Error(err)
		}
	}
	if _, err := safeTarget(context.Background(), "100.100.100.200:80", true); err != nil {
		t.Fatal(err)
	}
}

func TestSharedAddressDNSPolicy(t *testing.T) {
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			var size [2]byte
			if _, err := io.ReadFull(server, size[:]); err != nil {
				return
			}
			b := make([]byte, binary.BigEndian.Uint16(size[:]))
			if _, err := io.ReadFull(server, b); err != nil {
				return
			}
			end := 12
			for b[end] != 0 {
				end += int(b[end]) + 1
			}
			end += 5
			b = b[:end]
			typ := binary.BigEndian.Uint16(b[len(b)-4:])
			b[10], b[11] = 0, 0
			b[2], b[3] = 0x81, 0x80
			if typ == 1 {
				b[7] = 1
				b = append(b, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 100, 100, 100, 200)
			}
			binary.BigEndian.PutUint16(size[:], uint16(len(b)))
			server.Write(append(size[:], b...))
		}()
		return client, nil
	}}
	defer func() { net.DefaultResolver = old }()
	if _, err := safeTarget(context.Background(), "metadata.test:80", false); err == nil {
		t.Fatal("DNS shared address accepted")
	}
}

func TestBridgeResetCleanup(t *testing.T) {
	for _, reset := range []string{"tcp", "quic"} {
		t.Run(reset, func(t *testing.T) {
			client, server := quicPair(t)
			remote, err := client.OpenStreamSync(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			remote.Write([]byte{1})
			stream, err := server.AcceptStream(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			io.ReadFull(stream, make([]byte, 1))
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			peer, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			socket, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			done := make(chan struct{})
			go func() { bridge(socket, stream); close(done) }()
			if reset == "tcp" {
				peer.(*net.TCPConn).SetLinger(0)
				peer.Close()
			} else {
				remote.CancelWrite(9)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("bridge stuck after reset")
			}
			if client.Context().Err() != nil {
				t.Fatal("reset killed whole connection")
			}
		})
	}
}

func TestUDPStreamTermination(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "eof", true: "reset"}[reset], func(t *testing.T) {
			client, tunnel := quicPair(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := newServer("secret", "priority")
			s.agents["a"] = &agentPeer{id: "a", conn: tunnel}
			addr, err := s.listenSOCKS(ctx, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ended := make(chan struct{})
			go func() {
				q, err := client.AcceptStream(ctx)
				if err != nil {
					return
				}
				io.ReadFull(q, make([]byte, 1))
				q.Write([]byte{0})
				<-ended
				if reset {
					q.CancelWrite(9)
				} else {
					q.Close()
				}
			}()
			control := socks(t, addr, 3, "0.0.0.0:0")
			defer control.Close()
			reply(t, control)
			close(ended)
			control.SetReadDeadline(time.Now().Add(time.Second))
			_, err = control.Read(make([]byte, 1))
			if err == nil {
				t.Fatal("control alive")
			}
			if e, ok := err.(net.Error); ok && e.Timeout() {
				t.Fatal("control not closed after stream termination")
			}
			until := time.Now().Add(time.Second)
			for {
				count := 0
				s.agents["a"].udp.Range(func(_, _ any) bool { count++; return true })
				if count == 0 {
					break
				}
				if time.Now().After(until) {
					t.Fatal("relay registration leaked")
				}
				time.Sleep(time.Millisecond)
			}
			if client.Context().Err() != nil {
				t.Fatal("stream termination killed tunnel")
			}
		})
	}
}

func TestUDPOneWayIdleAndHardLifetime(t *testing.T) {
	client, server := quicPair(t)
	remote, err := client.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	remote.Write([]byte{3})
	stream, err := server.AcceptStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	io.ReadFull(stream, make([]byte, 1))
	done := make(chan struct{})
	go func() {
		agentUDPTimeouts(server, stream, true, 200*time.Millisecond, 900*time.Millisecond)
		close(done)
	}()
	if _, err := io.ReadFull(remote, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	a, _ := encodeAddress(sink.LocalAddr().String())
	packet := append(append([]byte{0, 0, 0}, a...), 1)
	key := udpKey{server, uint64(stream.StreamID())}
	v, ok := agentQueues.Load(key)
	if !ok {
		t.Fatal("missing queue")
	}
	start := time.Now()
	for time.Since(start) < 600*time.Millisecond {
		v.(chan []byte) <- packet
		sink.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		if _, _, err := sink.ReadFromUDP(make([]byte, 128)); err != nil {
			t.Fatalf("one-way traffic stopped: %v", err)
		}
		select {
		case <-done:
			t.Fatal("outbound traffic did not refresh idle")
		case <-time.After(40 * time.Millisecond):
		}
	}
	// Keep sending beyond hard lifetime; traffic must not extend it.
	for {
		select {
		case <-done:
			return
		case <-time.After(40 * time.Millisecond):
			v.(chan []byte) <- packet
		}
		if time.Since(start) > 1500*time.Millisecond {
			t.Fatal("hard lifetime extended by traffic")
		}
	}
}

func TestUDPMalformedDoesNotPinPort(t *testing.T) {
	st, ct := testTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	qa, err := s.listenQUIC(ctx, "127.0.0.1:0", st)
	if err != nil {
		t.Fatal(err)
	}
	sa, err := s.listenSOCKS(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go runAgent(ctx, qa, ct, "secret", "a", 1, true)
	waitAgents(t, s, 1)
	control := socks(t, sa, 3, "0.0.0.0:0")
	defer control.Close()
	relay, err := net.ResolveUDPAddr("udp", reply(t, control))
	if err != nil {
		t.Fatal(err)
	}
	bad, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	good, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		b := make([]byte, 128)
		n, from, err := echo.ReadFromUDP(b)
		if err == nil {
			echo.WriteToUDP(b[:n], from)
		}
	}()
	bad.WriteToUDP([]byte{0, 0, 1, 1}, relay)
	time.Sleep(30 * time.Millisecond)
	a, _ := encodeAddress(echo.LocalAddr().String())
	packet := append(append([]byte{0, 0, 0}, a...), 1)
	good.WriteToUDP(packet, relay)
	good.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := good.ReadFromUDP(make([]byte, 128)); err != nil {
		t.Fatalf("malformed packet pinned source: %v", err)
	}
}

func quicPair(t *testing.T) (*quic.Conn, *quic.Conn) {
	t.Helper()
	st, ct := testTLS(t)
	l, err := quic.ListenAddr("127.0.0.1:0", st, qc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := quic.DialAddr(ctx, l.Addr().String(), ct, qc)
	if err != nil {
		t.Fatal(err)
	}
	server, err := l.Accept(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.CloseWithError(0, "test"); server.CloseWithError(0, "test") })
	return client, server
}
