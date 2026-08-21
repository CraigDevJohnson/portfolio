package config

import "testing"

func TestLocalPortalPreviewRequiresExplicitTrue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "unset", value: "", want: false},
		{name: "false", value: "false", want: false},
		{name: "numeric true is rejected", value: "1", want: false},
		{name: "yes is rejected", value: "yes", want: false},
		{name: "true", value: "true", want: true},
		{name: "case insensitive true", value: " TRUE ", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MGMT_LOCAL_PREVIEW", test.value)
			if got := LocalPortalPreviewRequested(); got != test.want {
				t.Fatalf("LocalPortalPreviewRequested() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLocalPortalPreviewRequiresLoopbackListener(t *testing.T) {
	t.Setenv("MGMT_LOCAL_PREVIEW", "true")

	tests := []struct {
		name          string
		listenAddress string
		want          bool
	}{
		{name: "IPv4 loopback", listenAddress: "127.0.0.1:8080", want: true},
		{name: "IPv6 loopback", listenAddress: "[::1]:8080", want: true},
		{name: "localhost", listenAddress: "localhost:8080", want: true},
		{name: "all IPv4 interfaces", listenAddress: "0.0.0.0:8080", want: false},
		{name: "all IPv6 interfaces", listenAddress: "[::]:8080", want: false},
		{name: "external address", listenAddress: "192.0.2.10:8080", want: false},
		{name: "malformed address", listenAddress: "localhost", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := LocalPortalPreviewEnabled(test.listenAddress); got != test.want {
				t.Fatalf("LocalPortalPreviewEnabled(%q) = %v, want %v", test.listenAddress, got, test.want)
			}
		})
	}
}

func TestLocalPortalPreviewRemainsOffWithoutRequest(t *testing.T) {
	t.Setenv("MGMT_LOCAL_PREVIEW", "false")
	if LocalPortalPreviewEnabled("127.0.0.1:8080") {
		t.Fatal("expected local portal preview to remain disabled")
	}
}
