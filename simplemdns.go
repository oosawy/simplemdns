package simplemdns

import (
	"log/slog"
)

var logger = slog.Default().With("pkg", "simplemdns")

func SetLogger(l *slog.Logger) {
	if l != nil {
		logger = l.With("pkg", "simplemdns")
	}
}

// 9000 is the typical MTU of Ethernet minus some overhead.
var udpBufSize = 9000

func SetUDPBufferSize(size int) {
	if size < 512 || size > 9000 {
		logger.Warn("UDP buffer size outside recommended range for mDNS", slog.Int("size", size))
	}

	udpBufSize = size
}
