package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
)

const serviceType = "_emusync._tcp"

// Server is a host discovered via mDNS DNS-SD.
type Server struct {
	Host     string
	Port     int
	Instance string
}

// Advertise blocks until ctx is cancelled, announcing emusync on the LAN.
func Advertise(ctx context.Context, port int, instance string) error {
	if instance == "" {
		instance = "emusync"
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "emusync"
	}

	ips := localUnicastIPv4s()
	info := []string{"v=1", "app=emusync"}

	svc, err := mdns.NewMDNSService(instance, serviceType, "local.", fmt.Sprintf("%s.local.", host), port, ips, info)
	if err != nil {
		return fmt.Errorf("mdns service: %w", err)
	}
	srv, err := mdns.NewServer(&mdns.Config{Zone: svc})
	if err != nil {
		return fmt.Errorf("mdns server: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown()
	}()

	<-ctx.Done()
	return ctx.Err()
}

// Lookup listens for up to timeout for emusync servers.
func Lookup(parent context.Context, timeout time.Duration) []Server {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	entries := make(chan *mdns.ServiceEntry, 32)
	go func() {
		defer close(entries)
		// Lookup fills entries then returns; channel closed in defer above.
		if err := mdns.Lookup(serviceType, entries); err != nil {
			return
		}
	}()

	var mu sync.Mutex
	seen := map[string]struct{}{}
	var list []Server

	for {
		select {
		case <-ctx.Done():
			return list
		case e, ok := <-entries:
			if !ok {
				return list
			}
			if e == nil {
				continue
			}
			host := ""
			if e.AddrV4 != nil {
				host = e.AddrV4.String()
			} else if ip := e.Addr; ip != nil {
				if v4 := ip.To4(); v4 != nil {
					host = v4.String()
				}
			}
			if host == "" || e.Port == 0 {
				continue
			}
			key := fmt.Sprintf("%s:%d", host, e.Port)
			mu.Lock()
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				inst := strings.TrimSuffix(strings.TrimSpace(e.Name), ".")
				list = append(list, Server{Host: host, Port: e.Port, Instance: inst})
			}
			mu.Unlock()
		}
	}
}

func localUnicastIPv4s() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			out = append(out, ip)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
