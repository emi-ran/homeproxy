package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
)

var errAddressType = errors.New("address type")

func encodeAddress(addr string) ([]byte, error) {
	h, p, e := net.SplitHostPort(addr)
	if e != nil {
		return nil, e
	}
	port, e := strconv.Atoi(p)
	if e != nil || port < 0 || port > 65535 {
		return nil, errors.New("port")
	}
	var b []byte
	if ip := net.ParseIP(h); ip != nil {
		if v := ip.To4(); v != nil {
			b = append([]byte{1}, v...)
		} else {
			b = append([]byte{4}, ip.To16()...)
		}
	} else {
		if len(h) == 0 || len(h) > 255 {
			return nil, errors.New("host")
		}
		b = append([]byte{3, byte(len(h))}, []byte(h)...)
	}
	return append(b, byte(port>>8), byte(port)), nil
}

func readAddress(r io.Reader) (string, error) {
	b := make([]byte, 1)
	if _, e := io.ReadFull(r, b); e != nil {
		return "", e
	}
	n := 0
	domain := false
	switch b[0] {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		if _, e := io.ReadFull(r, b); e != nil {
			return "", e
		}
		n = int(b[0])
		domain = true
		if n == 0 {
			return "", errors.New("host")
		}
	default:
		return "", errAddressType
	}
	b = make([]byte, n+2)
	if _, e := io.ReadFull(r, b); e != nil {
		return "", e
	}
	h := string(b[:n])
	if !domain {
		h = net.IP(b[:n]).String()
	}
	return net.JoinHostPort(h, strconv.Itoa(int(binary.BigEndian.Uint16(b[n:])))), nil
}

func safeTarget(ctx context.Context, addr string, allow bool) (string, error) {
	h, p, e := net.SplitHostPort(addr)
	if e != nil || p == "0" {
		return "", errors.New("target")
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, h)
	if e != nil {
		return "", e
	}
	if len(ips) == 0 {
		return "", errors.New("dns")
	}
	for _, v := range ips {
		ip := v.IP
		v4 := ip.To4()
		shared := v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
		if !allow && (shared || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || !ip.IsGlobalUnicast()) {
			return "", errors.New("non-public destination blocked")
		}
	}
	return net.JoinHostPort(ips[0].IP.String(), p), nil
}
