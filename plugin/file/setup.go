package file

import (
	"os"
	"path/filepath"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/caddyserver/caddy"
)

func init() { plugin.Register("file", setup) }

func setup(c *caddy.Controller) error {
	zones, err := fileParse(c)
	if err != nil {
		return plugin.Error("file", err)
	}

	f := File{Zones: zones}
	// get the transfer plugin, so we can send notifies and send notifies on startup as well.
	c.OnStartup(func() error {
		for _, p := range dnsserver.GetConfig(c).Handlers() {
			if t, ok := p.(*transfer.Transfer); ok {
				f.transfer = t
				break
			}
		}
		if f.transfer == nil {
			return nil
		}
		for _, n := range zones.Names {
			f.transfer.Notify(n)
		}
		return nil
	})

	c.OnRestartFailed(func() error {
		for _, p := range dnsserver.GetConfig(c).Handlers() {
			if t, ok := p.(*transfer.Transfer); ok {
				f.transfer = t
				break
			}
		}
		if f.transfer == nil {
			return nil
		}
		for _, n := range zones.Names {
			f.transfer.Notify(n)
		}
		return nil
	})

	for _, n := range zones.Names {
		z := zones.Z[n]
		c.OnShutdown(z.OnShutdown)
		c.OnStartup(func() error {
			z.StartupOnce.Do(func() { z.Reload(f.transfer) })
			return nil
		})
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		f.Next = next
		return f
	})

	return nil
}

func fileParse(c *caddy.Controller) (Zones, error) {
	z := make(map[string]*Zone)
	names := []string{}

	config := dnsserver.GetConfig(c)

	var openErr error
	reload := 1 * time.Minute

	for c.Next() {
		// file db.file [zones...]
		if !c.NextArg() {
			return Zones{}, c.ArgErr()
		}
		fileName := c.Val()

		origins := make([]string, len(c.ServerBlockKeys))
		copy(origins, c.ServerBlockKeys)
		args := c.RemainingArgs()
		if len(args) > 0 {
			origins = args
		}

		if !filepath.IsAbs(fileName) && config.Root != "" {
			fileName = filepath.Join(config.Root, fileName)
		}

		reader, err := os.Open(fileName)
		if err != nil {
			openErr = err
		}

		for i := range origins {
			origins[i] = plugin.Host(origins[i]).Normalize()
			z[origins[i]] = NewZone(origins[i], fileName)
			if openErr == nil {
				reader.Seek(0, 0)
				zone, err := Parse(reader, origins[i], fileName, 0)
				if err == nil {
					z[origins[i]] = zone
				} else {
					return Zones{}, err
				}
			}
			names = append(names, origins[i])
		}

		t := []string{}
		var e error

		for c.NextBlock() {
			switch c.Val() {
			case "transfer":
				t, _, e = parse.Transfer(c, false)
				if e != nil {
					return Zones{}, e
				}

			case "reload":
				d, err := time.ParseDuration(c.RemainingArgs()[0])
				if err != nil {
					return Zones{}, plugin.Error("file", err)
				}
				reload = d

			case "upstream":
				// remove soon
				c.RemainingArgs()

			default:
				return Zones{}, c.Errf("unknown property '%s'", c.Val())
			}

			for _, origin := range origins {
				if t != nil {
					z[origin].TransferTo = append(z[origin].TransferTo, t...)
				}
			}
		}
	}

	for origin := range z {
		z[origin].ReloadInterval = reload
		z[origin].Upstream = upstream.New()
	}

	if openErr != nil {
		if reload == 0 {
			// reload hasn't been set make this a fatal error
			return Zones{}, plugin.Error("file", openErr)
		}
		log.Warningf("Failed to open %q: trying again in %s", openErr, reload)

	}
	return Zones{Z: z, Names: names}, nil
}
