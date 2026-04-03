package p2p

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

// LoadBootnodes reads bootnode multiaddrs from a YAML/text file.
// Each line is a multiaddr string (e.g., /ip4/1.2.3.4/udp/9000/quic-v1/p2p/QmPeer...).
// Matches ethlambda main.rs bootnode loading from nodes.yaml.
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
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			// Skip empty lines, comments, and YAML list markers.
			if strings.HasPrefix(line, "- ") {
				line = strings.TrimPrefix(line, "- ")
				line = strings.Trim(line, "\"' ")
			} else {
				continue
			}
		}

		if !strings.HasPrefix(line, "/") {
			continue // not a multiaddr
		}

		ma, err := multiaddr.NewMultiaddr(line)
		if err != nil {
			return nil, fmt.Errorf("parse bootnode multiaddr %q: %w", line, err)
		}
		addrs = append(addrs, ma)
	}

	return addrs, scanner.Err()
}
