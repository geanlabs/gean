package p2p

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

// LoadBootnodes reads bootnodes from a YAML/text file.
// Supports both multiaddr format (/ip4/...) and ENR format (enr:...).
func LoadBootnodes(path string) ([]multiaddr.Multiaddr, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open bootnodes file: %w", err)
	}
	defer f.Close()

	var addrs []multiaddr.Multiaddr
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip YAML list marker.
		if strings.HasPrefix(line, "- ") {
			line = strings.TrimPrefix(line, "- ")
			line = strings.Trim(line, "\"' ")
		}

		if line == "" {
			continue
		}

		// ENR format.
		if strings.HasPrefix(line, "enr:") {
			ma, err := ParseENR(line)
			if err != nil {
				return nil, fmt.Errorf("parse ENR bootnode: %w", err)
			}
			addrs = append(addrs, ma)
			continue
		}

		// Multiaddr format.
		if strings.HasPrefix(line, "/") {
			ma, err := multiaddr.NewMultiaddr(line)
			if err != nil {
				return nil, fmt.Errorf("parse bootnode multiaddr %q: %w", line, err)
			}
			addrs = append(addrs, ma)
			continue
		}

		// Skip unknown formats.
	}

	return addrs, scanner.Err()
}
