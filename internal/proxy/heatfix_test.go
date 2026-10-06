package proxy

import (
	"testing"
	"time"
)

// Guards for the phone-heat fixes. rusty_enet HOST_DEFAULT_MTU = 1392 (Mori never
// calls set_mtu) + 8 tunnel id + 10 SOCKS5 UDP header; anything smaller silently
// drops near-MTU ENet packets and reliable UDP retransmits them forever.

const enetMTU, tunnelID, socksUDP = 1392, 8, 10

func TestMaxDatagramClearsENetMTU(t *testing.T) {
	floor := enetMTU + tunnelID + socksUDP
	if maxDatagram < floor {
		t.Fatalf("maxDatagram=%d < ENet near-MTU packet %d: drops cause infinite retransmit", maxDatagram, floor)
	}
}

// Short QUIC keepalives keep the phone radio awake with zero data traffic.
func TestQUICRadioIdleTimings(t *testing.T) {
	if qc.KeepAlivePeriod < 30*time.Second {
		t.Fatalf("KeepAlivePeriod=%v: too short, radio never sleeps", qc.KeepAlivePeriod)
	}
	if qc.MaxIdleTimeout <= qc.KeepAlivePeriod {
		t.Fatalf("MaxIdleTimeout=%v must outlive KeepAlivePeriod=%v", qc.MaxIdleTimeout, qc.KeepAlivePeriod)
	}
}

// Fixed-interval retry re-runs a full TLS 1.3 handshake while the network is down.
func TestReconnectDelayBackoff(t *testing.T) {
	for fails := 0; fails < 10; fails++ {
		want := 5 * time.Second
		for i := 0; i < fails && want < time.Minute; i++ {
			want *= 2
		}
		if want > time.Minute {
			want = time.Minute
		}
		for n := 0; n < 32; n++ {
			d := reconnectDelay(fails)
			if d < want/2 || d > want {
				t.Fatalf("reconnectDelay(%d)=%v outside jitter range [%v,%v]", fails, d, want/2, want)
			}
		}
	}
}
