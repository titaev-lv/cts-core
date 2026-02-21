package config

import "testing"

func TestParseSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"plain", "1024", 1024, false},
		{"k", "64k", 64 * 1024, false},
		{"M", "1M", 1024 * 1024, false},
		{"negative", "-1", 0, true},
		{"invalid", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ParseSize() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHTTP2ConfigParse(t *testing.T) {
	cfg := &HTTP2Config{
		MaxConcurrentStreams:     "1000",
		InitialWindowSize:        "1M",
		MaxFrameSize:             "256k",
		MaxHeaderListSize:        "1M",
		IdleTimeoutSeconds:       120,
		MaxUploadBufferPerConn:   "1M",
		MaxUploadBufferPerStream: "1M",
	}

	parsed, err := cfg.Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.MaxConcurrentStreams != 1000 {
		t.Fatalf("MaxConcurrentStreams = %d, want 1000", parsed.MaxConcurrentStreams)
	}
}

func TestHTTP2ConfigParseInvalid(t *testing.T) {
	cfg := &HTTP2Config{MaxFrameSize: "invalid"}
	if _, err := cfg.Parse(); err == nil {
		t.Fatal("expected Parse() to fail for invalid size")
	}
}
