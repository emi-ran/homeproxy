package proxy

import (
	"context"
	"errors"
	"net"
	"strconv"
)

type ipLookup func(context.Context, string) ([]net.IPAddr, error)

func resolveUDPTarget(ctx context.Context, addr string, allow bool, lookup ipLookup) (*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, errors.New("target port")
	}
	ips, err := targetIPs(ctx, host, allow, lookup)
	if err != nil {
		return nil, err
	}
	return &net.UDPAddr{IP: ips[0].IP, Port: n}, nil
}
