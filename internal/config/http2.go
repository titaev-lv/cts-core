package config

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	lastChar := s[len(s)-1]
	var multiplier int64 = 1

	switch lastChar {
	case 'k', 'K':
		multiplier = 1024
		s = s[:len(s)-1]
	case 'm', 'M':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	}

	value, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size format: %w", err)
	}
	if value < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}

	result := value * multiplier
	if result > 100*1024*1024 {
		return 0, fmt.Errorf("size too large: %d bytes (max 100M)", result)
	}

	return result, nil
}

// HTTP2Config defines HTTP/2 server configuration.
type HTTP2Config struct {
	MaxConcurrentStreams     string `yaml:"max_concurrent_streams"`
	InitialWindowSize        string `yaml:"initial_window_size"`
	MaxFrameSize             string `yaml:"max_frame_size"`
	MaxHeaderListSize        string `yaml:"max_header_list_size"`
	IdleTimeoutSeconds       int    `yaml:"idle_timeout_seconds"`
	MaxUploadBufferPerConn   string `yaml:"max_upload_buffer_per_conn"`
	MaxUploadBufferPerStream string `yaml:"max_upload_buffer_per_stream"`
}

type ParsedHTTP2Config struct {
	MaxConcurrentStreams     uint32
	InitialWindowSize        int32
	MaxFrameSize             uint32
	MaxHeaderListSize        uint32
	IdleTimeoutSeconds       int
	MaxUploadBufferPerConn   int32
	MaxUploadBufferPerStream int32
}

func DefaultHTTP2Config() *HTTP2Config {
	return &HTTP2Config{
		MaxConcurrentStreams:     "250",
		InitialWindowSize:        "64k",
		MaxFrameSize:             "16k",
		MaxHeaderListSize:        "1M",
		IdleTimeoutSeconds:       120,
		MaxUploadBufferPerConn:   "1M",
		MaxUploadBufferPerStream: "1M",
	}
}

func (h *HTTP2Config) Parse() (*ParsedHTTP2Config, error) {
	parsed := &ParsedHTTP2Config{IdleTimeoutSeconds: h.IdleTimeoutSeconds}

	if h.MaxConcurrentStreams != "" {
		maxStreams, err := strconv.ParseUint(h.MaxConcurrentStreams, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid max_concurrent_streams: %w", err)
		}
		parsed.MaxConcurrentStreams = uint32(maxStreams)
	}

	if h.InitialWindowSize != "" {
		size, err := ParseSize(h.InitialWindowSize)
		if err != nil {
			return nil, fmt.Errorf("invalid initial_window_size: %w", err)
		}
		if size > 2147483647 {
			return nil, fmt.Errorf("initial_window_size exceeds int32 limit: %d", size)
		}
		parsed.InitialWindowSize = int32(size)
	}

	if h.MaxFrameSize != "" {
		size, err := ParseSize(h.MaxFrameSize)
		if err != nil {
			return nil, fmt.Errorf("invalid max_frame_size: %w", err)
		}
		if size > 4294967295 {
			return nil, fmt.Errorf("max_frame_size exceeds uint32 limit: %d", size)
		}
		parsed.MaxFrameSize = uint32(size)
	}

	if h.MaxHeaderListSize != "" {
		size, err := ParseSize(h.MaxHeaderListSize)
		if err != nil {
			return nil, fmt.Errorf("invalid max_header_list_size: %w", err)
		}
		if size > 4294967295 {
			return nil, fmt.Errorf("max_header_list_size exceeds uint32 limit: %d", size)
		}
		parsed.MaxHeaderListSize = uint32(size)
	}

	if h.MaxUploadBufferPerConn != "" {
		size, err := ParseSize(h.MaxUploadBufferPerConn)
		if err != nil {
			return nil, fmt.Errorf("invalid max_upload_buffer_per_conn: %w", err)
		}
		if size > 2147483647 {
			return nil, fmt.Errorf("max_upload_buffer_per_conn exceeds int32 limit: %d", size)
		}
		parsed.MaxUploadBufferPerConn = int32(size)
	}

	if h.MaxUploadBufferPerStream != "" {
		size, err := ParseSize(h.MaxUploadBufferPerStream)
		if err != nil {
			return nil, fmt.Errorf("invalid max_upload_buffer_per_stream: %w", err)
		}
		if size > 2147483647 {
			return nil, fmt.Errorf("max_upload_buffer_per_stream exceeds int32 limit: %d", size)
		}
		parsed.MaxUploadBufferPerStream = int32(size)
	}

	if parsed.IdleTimeoutSeconds < 0 || parsed.IdleTimeoutSeconds > 3600 {
		return nil, fmt.Errorf("invalid idle_timeout_seconds: %d", parsed.IdleTimeoutSeconds)
	}

	return parsed, nil
}
