package transfer

// from older file code, see if we need this

/*
// ServeIxfr checks if we need to serve a simpler IXFR for the incoming message.
// See RFC 1995 Section 3: "... and the authority section containing the SOA record of client's version of the zone."
// and Section 2, paragraph 4 where we only need to echo the SOA record back.
// This function must be called when the qtype is IXFR. It returns a plugin.ClientWrite(code) == false, when it didn't
// write anything and we should perform an AXFR.
func (x Xfr) ServeIxfr(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Ns) != 1 {
		return dns.RcodeServerFailure, nil
	}
	soa, ok := r.Ns[0].(*dns.SOA)
	if !ok {
		return dns.RcodeServerFailure, nil
	}

	x.RLock()
	if x.Apex.SOA == nil {
		x.RUnlock()
		return dns.RcodeServerFailure, nil
	}
	serial := x.Apex.SOA.Serial
	x.RUnlock()

	if soa.Serial == serial { // Section 2, para 4; echo SOA back. We have the same zone
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{soa}
		w.WriteMsg(m)
		return 0, nil
	}
	return dns.RcodeServerFailure, nil
}
*/
