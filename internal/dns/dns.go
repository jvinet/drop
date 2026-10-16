// Copyright 2026 Jan Wrobel <jan@mixedbit.org>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dns

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/miekg/dns" // Until v2 is stable

	"github.com/wrr/drop/internal/log"
)

const upstreamTimeout = 5 * time.Second
const ttl = 5

type server struct {
	resolver *net.Resolver
}

// StartServer answers queries arriving on conn until the returned
// shutdown function is called. Names are resolved with the name
// servers configured on the host.
//
// Takes ownership of conn, closing it when the server fails to start
// or the shutdown function is called.
func StartServer(conn net.PacketConn) (func(), error) {
	return startServer(conn, "")
}

// startServer allows to set an upstream server where DNS lookups are
// send instead of using the host-configured upstreams. This is needed
// for unit tests.
func startServer(conn net.PacketConn, upstream string) (func(), error) {
	resolver := &net.Resolver{PreferGo: true}
	if upstream != "" {
		resolver.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, upstream)
		}
	}
	s := &server{resolver: resolver}
	srv := &dns.Server{
		PacketConn: conn,
		Handler:    dns.HandlerFunc(s.handle),
	}

	started := make(chan struct{})
	srv.NotifyStartedFunc = func() { close(started) }
	errs := make(chan error, 1)
	go func() { errs <- srv.ActivateAndServe() }()
	select {
	case <-started:
	case err := <-errs:
		conn.Close()
		return nil, err
	}

	return func() { srv.Shutdown() }, nil
}

func (s *server) handle(w dns.ResponseWriter, req *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetReply(req)
	reply.RecursionAvailable = true

	if len(req.Question) != 1 {
		// Accept only allows requests with question count set to 1 in the
		// header, but it allows requests that end just after the header,
		// without including any questions.
		info("format error, no question in request")
		reply.Rcode = dns.RcodeFormatError
		if err := w.WriteMsg(reply); err != nil {
			info("send answer: %v", err)
		}
		return
	}
	question := req.Question[0]
	info("query %s %s", dns.TypeToString[question.Qtype], question.Name)

	switch {
	case !req.RecursionDesired:
		// The host resolver always recurses.
		info("refused %s, recursion not desired", question.Name)
		reply.Rcode = dns.RcodeRefused
	case question.Qtype == dns.TypeA:
		s.resolve(reply, question)
	case question.Qtype == dns.TypeAAAA:
		// No IPv6 support with firewall yet. An empty NOERROR keeps
		// the client on A, REFUSED would make it fail the lookup.
	default:
		// Also an empty NOERROR: clients fall back to A or give up,
		// REFUSED can be treated as a server problem and
		// can lead to retriers.
		info("no data for %s query for %s", dns.TypeToString[question.Qtype], question.Name)
	}

	minClientSize := dns.MinMsgSize
	if opt := req.IsEdns0(); opt != nil {
		// A reply must carry an OPT record if the query did.

		// We just advertise the default UDP max size of 512, because we
		// expect just a single query, but any larger size will work
		// without fragmentation, as this is a local communication only.
		reply.SetEdns0(dns.MinMsgSize, false)
		minClientSize = int(opt.UDPSize())
	}
	// Drops records that do not fit the client buffer and sets the
	// truncated bit. Enables compression when needed.
	reply.Truncate(minClientSize)

	if err := w.WriteMsg(reply); err != nil {
		info("send answer: %v", err)
	}
}

// resolve fills in reply for an address query.
func (s *server) resolve(reply *dns.Msg, question dns.Question) {
	ctx, cancel := context.WithTimeout(context.Background(), upstreamTimeout)
	defer cancel()

	// The incoming name is always rooted, CanonicalName ensures it is
	// also lowercase, so the firewall always sees rooted, lowercase
	// names.
	addrs, err := s.resolver.LookupNetIP(ctx, "ip4", dns.CanonicalName(question.Name))
	if err != nil {
		var dnsErr *net.DNSError
		switch {
		case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
			reply.Rcode = dns.RcodeNameError
		default:
			info("resolve %s: %v", question.Name, err)
			reply.Rcode = dns.RcodeServerFailure
		}
		return
	}

	header := dns.RR_Header{
		Name:   question.Name,
		Rrtype: question.Qtype,
		Class:  dns.ClassINET,
		Ttl:    ttl,
	}
	var answered []netip.Addr
	for _, addr := range addrs {
		addr = addr.Unmap()
		if !addr.Is4() {
			// We support ipv4 only
			continue
		}
		reply.Answer = append(reply.Answer, &dns.A{Hdr: header, A: addr.AsSlice()})
		answered = append(answered, addr)
	}
	info("answer %s %s", question.Name, formatAddrs(answered))
}

func info(format string, args ...any) {
	log.Info("dns: "+format, args...)
}

func formatAddrs(addrs []netip.Addr) string {
	if len(addrs) == 0 {
		return "no addresses"
	}
	s := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		s = append(s, addr.String())
	}
	return strings.Join(s, " ")
}
