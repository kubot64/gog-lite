package cmd

import (
	"fmt"
	"io"
	"os"
)

const maxStdinBytes = 10 * 1024 * 1024

func readStdinWithLimit(limit int64) (string, error) {
	if limit <= 0 {
		return "", fmt.Errorf("invalid stdin limit: %d", limit)
	}

	limited := io.LimitReader(os.Stdin, limit+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}

	if int64(len(b)) > limit {
		return "", fmt.Errorf("stdin exceeds %d bytes", limit)
	}

	return string(b), nil
}
