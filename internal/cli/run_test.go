package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wrr/drop/internal/config"
)

func TestFlagsToConfig(t *testing.T) {
	baseConfig := func() *config.Config {
		return &config.Config{
			Runtime: config.RuntimeNative,
			Mounts:  []config.Mount{{Source: "~/a", Target: "~/a"}},
			Net: config.Net{
				Mode: config.NetModeIsolated,
				TCPPublishedPorts: []config.PublishedPort{
					{HostIP: "127.0.0.1", HostPort: 8080, GuestPort: 8080},
				},
			},
		}
	}

	tests := []struct {
		name     string
		flags    runFlags
		expected *config.Config
		error    string
	}{
		{
			name:     "no flags",
			flags:    runFlags{},
			expected: baseConfig(),
		},
		{
			name: "mounts and ports are appended",
			flags: runFlags{
				Mounts:            []string{"~/b:~/c:rw"},
				TcpPublishedPorts: []string{"9090:90"},
				TcpHostPorts:      []string{"5432"},
				UdpPublishedPorts: []string{"5353"},
				UdpHostPorts:      []string{"53:5353"},
			},
			expected: &config.Config{
				Runtime: config.RuntimeNative,
				Mounts: []config.Mount{
					{Source: "~/a", Target: "~/a"},
					{Source: "~/b", Target: "~/c", RW: true},
				},
				Net: config.Net{
					Mode: config.NetModeIsolated,
					TCPPublishedPorts: []config.PublishedPort{
						{HostIP: "127.0.0.1", HostPort: 8080, GuestPort: 8080},
						{HostIP: "127.0.0.1", HostPort: 9090, GuestPort: 90},
					},
					TCPHostPorts: []config.HostPort{
						{HostPort: 5432, GuestPort: 5432},
					},
					UDPPublishedPorts: []config.PublishedPort{
						{HostIP: "127.0.0.1", HostPort: 5353, GuestPort: 5353},
					},
					UDPHostPorts: []config.HostPort{
						{HostPort: 53, GuestPort: 5353},
					},
				},
			},
		},
		{
			name:  "net mode and runtime are overridden",
			flags: runFlags{NetworkMode: config.NetModeOff, Runtime: config.RuntimeGvisor},
			expected: &config.Config{
				Runtime: config.RuntimeGvisor,
				Mounts:  []config.Mount{{Source: "~/a", Target: "~/a"}},
				Net: config.Net{
					Mode: config.NetModeOff,
					TCPPublishedPorts: []config.PublishedPort{
						{HostIP: "127.0.0.1", HostPort: 8080, GuestPort: 8080},
					},
				},
			},
		},
		{
			name:  "invalid port flag",
			flags: runFlags{TcpPublishedPorts: []string{"bogus"}},
			error: "command line --tcp-publish flag",
		},
		{
			name:  "invalid resulting config",
			flags: runFlags{NetworkMode: "bogus"},
			error: "command line flags: invalid network mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			err := flagsToConfig(cfg, &tt.flags)
			if tt.error != "" {
				if err == nil || !strings.Contains(err.Error(), tt.error) {
					t.Fatalf("expected error containing %q, got: %v", tt.error, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("expected %+v, got %+v", tt.expected, cfg)
			}
		})
	}
}
