package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/quic-go/quic-go"
	"testing"
	"time"
)

// Observe the real library limit without changing production MTU settings.
func TestQUICCurrentDatagramLimit(t *testing.T) {
	client, _ := quicPair(t)
	err := client.SendDatagram(make([]byte, maxDatagram))
	var large *quic.DatagramTooLargeError
	if err == nil {
		t.Logf("current localhost QUIC accepted application ceiling=%d (enqueue, not delivery proof)", maxDatagram)
	} else if errors.As(err, &large) {
		t.Logf("current localhost QUIC payload limit=%d, application ceiling=%d", large.MaxDatagramPayloadSize, maxDatagram)
	} else {
		t.Fatalf("unexpected SendDatagram error: %v", err)
	}
}

// This is a delivery diagnostic, not a promise for arbitrary mobile paths.
// Small bidirectional traffic gives transport PMTU probing time to converge.
func TestQUICNearMTUDeliveryDiagnostic(t *testing.T) {
	client, server := quicPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for i := 0; i < 50; i++ {
		for _, pair := range [][2]*quic.Conn{{client, server}, {server, client}} {
			if err := pair[0].SendDatagram([]byte{1}); err != nil {
				t.Fatal(err)
			}
			if _, err := pair[1].ReceiveDatagram(ctx); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, pair := range [][2]*quic.Conn{{client, server}, {server, client}} {
		payload := bytes.Repeat([]byte{0x5a}, 1410)
		err := pair[0].SendDatagram(payload)
		var large *quic.DatagramTooLargeError
		if errors.As(err, &large) {
			t.Logf("near-MTU unavailable after traffic: max payload=%d", large.MaxDatagramPayloadSize)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got, err := pair[1].ReceiveDatagram(ctx)
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("accepted near-MTU payload not delivered intact: len=%d err=%v", len(got), err)
		}
		t.Log("1410-byte near-MTU datagram delivered intact after PMTU warmup")
	}
}

func TestUDPStatsTypedSizeErrorAccounting(t *testing.T) {
	var s udpStats
	s.send(func(b []byte) error {
		if len(b) != 1410 {
			t.Fatalf("tunnel payload=%d", len(b))
		}
		return fmt.Errorf("wrapped: %w", &quic.DatagramTooLargeError{MaxDatagramPayloadSize: 1243})
	}, 7, make([]byte, 1402), true)
	v := s.snapshot()
	if v.SendTooLarge != 1 || v.LastMaxPayload != 1243 || v.SendOtherErrors != 0 || v.LocalOversize != 0 || v.ToAgentPackets != 0 || v.ToAgentBytes != 0 || v.SendCalls != 1 {
		t.Fatalf("typed size error accounting: %+v", v)
	}
}

func TestUDPStatsCountsBothDirections(t *testing.T) {
	var s udpStats
	send := func([]byte) error { return nil }
	s.send(send, 1, []byte{1}, true)
	s.send(send, 1, []byte{1, 2}, false)
	s.received(9, true)
	s.received(10, false)
	v := s.snapshot()
	if v.ToAgentPackets != 1 || v.ToAgentBytes != 9 || v.ToServerPackets != 1 || v.ToServerBytes != 10 || v.AtServerPackets != 1 || v.AtServerBytes != 9 || v.AtAgentPackets != 1 || v.AtAgentBytes != 10 {
		t.Fatalf("direction counters: %+v", v)
	}
}

func TestUDPStatsSeparatesLossAndDirection(t *testing.T) {
	var s udpStats
	q := make(chan []byte, 1)
	s.enqueue(q, []byte{1}, true)
	s.enqueue(q, []byte{2}, true)
	s.enqueue(q, []byte{3}, false)
	s.send(func([]byte) error { t.Fatal("oversize reached sender"); return nil }, 1, make([]byte, maxDatagram), true)
	s.send(func([]byte) error { return &quic.DatagramTooLargeError{MaxDatagramPayloadSize: 1243} }, 1, []byte{1}, true)
	s.send(func([]byte) error { return errors.New("closed") }, 1, []byte{1}, false)
	s.send(func([]byte) error { time.Sleep(time.Millisecond); return nil }, 1, []byte{1, 2}, false)
	v := s.snapshot()
	if v.ServerQueueDrops != 1 || v.AgentQueueDrops != 1 || v.LocalOversize != 1 || v.SendTooLarge != 1 || v.SendOtherErrors != 1 || v.LastMaxPayload != 1243 {
		t.Fatalf("loss counters: %+v", v)
	}
	if v.ToAgentPackets != 0 || v.ToServerPackets != 1 || v.ToServerBytes != 10 || v.SendCalls != 3 || v.SendNanoseconds == 0 {
		t.Fatalf("traffic counters: %+v", v)
	}
}
