// nativemsg/protocol.go — Chrome Native Messaging wire protocol.
//
// Chrome uses a simple framing: 4-byte little-endian uint32 length prefix,
// followed by a UTF-8 JSON payload. Max message size is 1 MB.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging
package nativemsg

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MaxMessageSize is Chrome's native messaging limit (1 MB).
const MaxMessageSize = 1024 * 1024

// ReadMessage reads one native messaging frame from r.
// Returns the raw JSON payload (without the length prefix).
func ReadMessage(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, fmt.Errorf("read length prefix: %w", err)
	}
	if length == 0 {
		return nil, fmt.Errorf("empty message (length=0)")
	}
	if length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, MaxMessageSize)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read payload (%d bytes): %w", length, err)
	}
	return buf, nil
}

// WriteMessage writes one native messaging frame to w.
// Prepends the 4-byte little-endian length prefix before the JSON payload.
func WriteMessage(w io.Writer, msg []byte) error {
	if len(msg) > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max %d)", len(msg), MaxMessageSize)
	}
	length := uint32(len(msg))
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return fmt.Errorf("write length prefix: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}
