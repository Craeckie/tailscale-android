// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package wedgewatch

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// ProbeName is answered by tailscale's resolver from resolveLocal with
// 100.100.100.100, without a netmap and without forwarding upstream, so a
// probe for it measures only the path through netstack and back.
const ProbeName = "magicdns.localhost-tailscale-daemon."

// QueryA sends one A query for ProbeName to server over UDP and checks that
// the answer carries want. The socket is a plain one (no netns binding), so
// on Android it is routed into the app's own VPN like any other app's.
func QueryA(ctx context.Context, server netip.AddrPort, want netip.Addr) error {
	var d net.Dialer
	c, err := d.DialContext(ctx, "udp", server.String())
	if err != nil {
		return err
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		c.SetDeadline(dl)
	} else {
		c.SetDeadline(time.Now().Add(5 * time.Second))
	}

	var idb [2]byte
	rand.Read(idb[:])
	id := binary.BigEndian.Uint16(idb[:])
	q, err := buildQuery(id)
	if err != nil {
		return err
	}
	if _, err := c.Write(q); err != nil {
		return err
	}
	buf := make([]byte, 1500)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return err
		}
		ok, err := checkAnswer(buf[:n], id, want)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		// A stray datagram with another ID: keep reading until the deadline.
	}
}

// The DNS wire format is built and parsed by hand (RFC 1035 §4) to keep
// golang.org/x/net out of the direct requirements in go.mod, an upstream-owned
// file every merge touches.

func buildQuery(id uint16) ([]byte, error) {
	q := make([]byte, 12, 64)
	binary.BigEndian.PutUint16(q[0:], id)
	q[2] = 0x01                          // RD
	binary.BigEndian.PutUint16(q[4:], 1) // QDCOUNT
	for _, label := range strings.Split(strings.TrimSuffix(ProbeName, "."), ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil, fmt.Errorf("bad label %q", label)
		}
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	q = append(q, 0, 0, typeA, 0, classIN)
	return q, nil
}

const (
	typeA   = 1
	classIN = 1
)

var errShort = errors.New("short DNS message")

// skipName returns the offset just past the (possibly compressed) name at off.
func skipName(msg []byte, off int) (int, error) {
	for {
		if off >= len(msg) {
			return 0, errShort
		}
		l := int(msg[off])
		switch {
		case l == 0:
			return off + 1, nil
		case l&0xC0 == 0xC0: // compression pointer ends the name
			if off+2 > len(msg) {
				return 0, errShort
			}
			return off + 2, nil
		case l&0xC0 != 0:
			return 0, fmt.Errorf("bad label type %#x", l)
		}
		off += 1 + l
	}
}

// checkAnswer reports whether msg answers query id; it returns an error if
// it does but the answer is not want.
func checkAnswer(msg []byte, id uint16, want netip.Addr) (bool, error) {
	if len(msg) < 12 || binary.BigEndian.Uint16(msg[0:]) != id || msg[2]&0x80 == 0 {
		return false, nil
	}
	if rcode := msg[3] & 0x0F; rcode != 0 {
		return true, fmt.Errorf("rcode %d", rcode)
	}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	an := int(binary.BigEndian.Uint16(msg[6:]))
	off := 12
	var err error
	for range qd {
		if off, err = skipName(msg, off); err != nil {
			return true, err
		}
		off += 4 // QTYPE, QCLASS
	}
	for range an {
		if off, err = skipName(msg, off); err != nil {
			return true, err
		}
		if off+10 > len(msg) {
			return true, errShort
		}
		typ := binary.BigEndian.Uint16(msg[off:])
		class := binary.BigEndian.Uint16(msg[off+2:])
		rdlen := int(binary.BigEndian.Uint16(msg[off+8:]))
		off += 10
		if off+rdlen > len(msg) {
			return true, errShort
		}
		if typ == typeA && class == classIN && rdlen == 4 && netip.AddrFrom4([4]byte(msg[off:off+4])) == want {
			return true, nil
		}
		off += rdlen
	}
	return true, fmt.Errorf("no A %v in answer", want)
}
