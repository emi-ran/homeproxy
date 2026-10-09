package proxy

import (
	"encoding/binary"
	"errors"
	"github.com/quic-go/quic-go"
	"sync/atomic"
	"time"
)

// UDPStatsSnapshot contains process-wide counters, not payloads or destination labels.
// Loads are individually atomic, not a transactional view. Bytes include the tunnel ID.
type UDPStatsSnapshot struct {
	ServerQueueDrops, AgentQueueDrops, LocalOversize, SendTooLarge, SendOtherErrors uint64
	SourceRejects                                                                   uint64
	LastMaxPayload                                                                  int64
	ToAgentPackets, ToAgentBytes, ToServerPackets, ToServerBytes                    uint64
	SendCalls, SendNanoseconds                                                      uint64
	AtServerPackets, AtServerBytes, AtAgentPackets, AtAgentBytes                    uint64
}
type udpStats struct {
	sourceRejects                                                                   atomic.Uint64
	atServerPackets, atServerBytes, atAgentPackets, atAgentBytes                    atomic.Uint64
	serverQueueDrops, agentQueueDrops, localOversize, sendTooLarge, sendOtherErrors atomic.Uint64
	lastMaxPayload                                                                  atomic.Int64
	toAgentPackets, toAgentBytes, toServerPackets, toServerBytes                    atomic.Uint64
	sendCalls, sendNanoseconds                                                      atomic.Uint64
}

func (s *udpStats) received(n int, server bool) {
	if server {
		s.atServerPackets.Add(1)
		s.atServerBytes.Add(uint64(n))
	} else {
		s.atAgentPackets.Add(1)
		s.atAgentBytes.Add(uint64(n))
	}
}

var udpMetrics udpStats

// SnapshotUDPStats is internal/local consumption only; no network endpoint is registered.
func SnapshotUDPStats() UDPStatsSnapshot { return udpMetrics.snapshot() }
func (s *udpStats) snapshot() UDPStatsSnapshot {
	return UDPStatsSnapshot{
		SourceRejects:   s.sourceRejects.Load(),
		AtServerPackets: s.atServerPackets.Load(), AtServerBytes: s.atServerBytes.Load(), AtAgentPackets: s.atAgentPackets.Load(), AtAgentBytes: s.atAgentBytes.Load(),
		ServerQueueDrops: s.serverQueueDrops.Load(), AgentQueueDrops: s.agentQueueDrops.Load(),
		LocalOversize: s.localOversize.Load(), SendTooLarge: s.sendTooLarge.Load(), SendOtherErrors: s.sendOtherErrors.Load(), LastMaxPayload: s.lastMaxPayload.Load(),
		ToAgentPackets: s.toAgentPackets.Load(), ToAgentBytes: s.toAgentBytes.Load(), ToServerPackets: s.toServerPackets.Load(), ToServerBytes: s.toServerBytes.Load(),
		SendCalls: s.sendCalls.Load(), SendNanoseconds: s.sendNanoseconds.Load(),
	}
}
func (s *udpStats) enqueue(q chan []byte, b []byte, server bool) {
	select {
	case q <- b:
	default:
		if server {
			s.serverQueueDrops.Add(1)
		} else {
			s.agentQueueDrops.Add(1)
		}
	}
}
func (s *udpStats) send(send func([]byte) error, id uint64, b []byte, toAgent bool) {
	if len(b)+8 > maxDatagram {
		s.localOversize.Add(1)
		return
	}
	d := make([]byte, len(b)+8)
	binary.BigEndian.PutUint64(d, id)
	copy(d[8:], b)
	start := time.Now()
	err := send(d)
	s.sendCalls.Add(1)
	s.sendNanoseconds.Add(uint64(time.Since(start)))
	if err != nil {
		var large *quic.DatagramTooLargeError
		if errors.As(err, &large) {
			s.sendTooLarge.Add(1)
			s.lastMaxPayload.Store(large.MaxDatagramPayloadSize)
		} else {
			s.sendOtherErrors.Add(1)
		}
		return
	}
	if toAgent {
		s.toAgentPackets.Add(1)
		s.toAgentBytes.Add(uint64(len(d)))
	} else {
		s.toServerPackets.Add(1)
		s.toServerBytes.Add(uint64(len(d)))
	}
}
