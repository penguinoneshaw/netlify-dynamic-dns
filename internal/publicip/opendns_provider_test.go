package publicip

import (
	"net"
	"testing"
)

func TestNewOpenDNSProvider(t *testing.T) {
	type args struct {
		coreResolver string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{name: "basic", args: args{coreResolver: cloudflareResolver}, want: true, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewOpenDNSProvider(tt.args.coreResolver)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOpenDNSProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got != nil) != tt.want {
				t.Errorf("NewOpenDNSProvider() = %v, want %v", got, tt.want)
			}

			if tt.want == true {
				if got.IPv4Addr.To4() == nil {
					t.Errorf("Expected %v to be an IPv4 address", got.IPv4Addr.String())
				}
			}
		})
	}
}

func TestOpenDNSProvider_GetIPv4(t *testing.T) {
	opdns, err := NewOpenDNSProvider(cloudflareResolver)
	if err != nil {
		t.Fatalf("Setting up DNS failed with error %v", err)
	}
	tests := []struct {
		name    string
		opdns   OpenDNSProvider
		wantErr bool
	}{
		{name: "basic", opdns: *opdns, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.opdns.GetIPv4()
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenDNSProvider.GetIPv4() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if ip := net.ParseIP(got); ip.To4() == nil {
					t.Errorf("Expected %v to be an IPv4 address", got)
				}
			}

		})
	}
}
