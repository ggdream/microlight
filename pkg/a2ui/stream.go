package a2ui

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

// ReadJSONL decodes one message per line and hands it to receive in arrival order.
func ReadJSONL(reader io.Reader, receive func(AgentMessage) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 16<<10), 4<<20)
	for line := 1; scanner.Scan(); line++ {
		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}
		message, err := Decode(data)
		if err != nil {
			return fmt.Errorf("A2UI line %d: %w", line, err)
		}
		if err := receive(message); err != nil {
			return fmt.Errorf("A2UI line %d: %w", line, err)
		}
	}
	return scanner.Err()
}
