package logger

import "go.uber.org/zap"

const maxDumpBytes = 4 << 10 // cap request/response dumps at 4 KiB

// RequestID is the field used to correlate all log lines of one request.
func RequestID(val string) zap.Field {
	return zap.String("request_id", val)
}

// RequestDump logs a (truncated) request body.
func RequestDump(body []byte) zap.Field {
	return zap.ByteString("request_body", truncate(body))
}

// ResponseDump logs a (truncated) response body.
func ResponseDump(body []byte) zap.Field {
	return zap.ByteString("response_body", truncate(body))
}

func truncate(b []byte) []byte {
	if len(b) <= maxDumpBytes {
		return b
	}

	return b[:maxDumpBytes]
}
