package transfer

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("transfer")

// Transfer is a plugin that handles zone transfers.
type Transfer struct {
	Transferers []Transferer // List of plugins that implement Transferer
	xfrs        []*xfr
	Next        plugin.Handler
}

type xfr struct {
	Zones []string
	to    []string
}

// Transferer may be implemented by plugins to enable zone transfers
type Transferer interface {
	// Transfer returns a channel to which it writes responses to the transfer request.
	// If the plugin is not authoritative for the zone, it should immediately return the
	// Transfer.ErrNotAuthoritative error.
	//
	// If serial is 0, handle as an AXFR request. Transfer should send all records
	// in the zone to the channel. The SOA should be written to the channel first, followed
	// by all other records, including all NS + glue records. The implemenation is also responsible
	// for sending the last SOA record (to signal end of the transfer). This plugin will just grab
	// these records and send them back to the requester, there is little validation done.
	//
	// If serial is not 0, it will be handled as an IXFR request. If the serial is equal to or greater (newer) than
	// the current serial for the zone, send a single SOA record to the channel and then close it.
	// If the serial is less (older) than the current serial for the zone, perform an AXFR fallback
	// by proceeding as if an AXFR was requested (as above).
	Transfer(zone string, serial uint32) (<-chan []dns.RR, error)
}

var (
	// ErrNotAuthoritative is returned by Transfer() when the plugin is not authoritative for the zone.
	ErrNotAuthoritative = errors.New("not authoritative for zone")
)

// From file transfer code:
/*
	// For IXFR we take the SOA in the IXFR message (if there), compare it what we have and then decide to do an
	// AXFR or just reply with one SOA message back.
	if state.QType() == dns.TypeIXFR {
		code, _ := x.ServeIxfr(ctx, w, r)
		if plugin.ClientWrite(code) {
			return code, nil
		}
	}
*/

// ServeDNS implements the plugin.Handler interface.
func (t *Transfer) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QType() != dns.TypeAXFR && state.QType() != dns.TypeIXFR {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	// Find the first transfer instance for which the queried zone is a subdomain.
	var x *xfr
	for _, xfr := range t.xfrs {
		zone := plugin.Zones(xfr.Zones).Matches(state.Name())
		if zone == "" {
			continue
		}
		x = xfr
	}
	if x == nil {
		// Requested zone did not match any transfer instance zones.
		// Pass request down chain in case later plugins are capable of handling transfer requests themselves.
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	if !x.allowed(state) {
		return dns.RcodeRefused, nil
	}

	// Get serial from request if this is an IXFR.
	var serial uint32
	if state.QType() == dns.TypeIXFR {
		if len(r.Ns) != 1 {
			return dns.RcodeServerFailure, nil
		}
		soa, ok := r.Ns[0].(*dns.SOA)
		if !ok {
			return dns.RcodeServerFailure, nil
		}
		serial = soa.Serial
	}

	// Get a receiving channel from the first Transferer plugin that returns one.
	var pchan <-chan []dns.RR
	var err error
	for _, p := range t.Transferers {
		pchan, err = p.Transfer(state.QName(), serial)
		if err == ErrNotAuthoritative {
			// plugin was not authoritative for the zone, try next plugin
			continue
		}
		if err != nil {
			return dns.RcodeServerFailure, err
		}
		break
	}

	if pchan == nil {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	// Send response to client
	ch := make(chan *dns.Envelope)
	tr := new(dns.Transfer)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		tr.Out(w, r, ch)
		wg.Done()
	}()

	rrs := []dns.RR{}
	l := 0
	var soa *dns.SOA
	for records := range pchan {
		if x, ok := records[0].(*dns.SOA); ok && soa == nil {
			soa = x
		}
		rrs = append(rrs, records...)
		if len(rrs) > 500 {
			ch <- &dns.Envelope{RR: rrs}
			l += len(rrs)
			rrs = []dns.RR{}
		}
	}

	// if we are here and we only hold 1 soa (len(rrs) == 1) and soa != nil, and IXFR fallback should
	// be performed. We haven't send anything on ch yet, so that can be closed (and waited for), and we only
	// need to return the SOA back to the client and return.
	if len(rrs) == 1 && soa != nil { // soa should never be nil...
		close(ch)
		wg.Wait()

		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{soa}
		w.WriteMsg(m)

		log.Infof("Outgoing incremental transfer for up to date zone %q to %s for %d SOA serial", state.QName(), state.IP(), serial)
		return 0, nil
	}

	if len(rrs) > 0 {
		ch <- &dns.Envelope{RR: rrs}
		l += len(rrs)
		rrs = []dns.RR{}
	}

	close(ch) // Even though we close the channel here, we still have
	wg.Wait() // to wait before we can return and close the connection.

	log.Infof("Outgoing transfer of %d records of zone %q to %s for %d SOA serial", l, state.QName(), state.IP(), serial)
	return 0, nil
}

func (x xfr) allowed(state request.Request) bool {
	for _, h := range x.to {
		if h == "*" {
			return true
		}
		to, _, err := net.SplitHostPort(h)
		if err != nil {
			return false
		}
		// If remote IP matches we accept.
		remote := state.IP()
		if to == remote {
			return true
		}
	}
	return false
}

// Name implements the Handler interface.
func (Transfer) Name() string { return "transfer" }
