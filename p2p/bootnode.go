package p2p

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

func LoadBootnodes(path string) ([]multiaddr.Multiaddr, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open bootnodes file: %w", err)
	}
	defer f.Close()

	var addrs []multiaddr.Multiaddr
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- ") {
			line = strings.TrimPrefix(line, "- ")
			line = strings.Trim(line, "\"' ")
		}

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "enr:") {
			ma, err := ParseENR(line)
			if err != nil {
				return nil, fmt.Errorf("parse ENR bootnode: %w", err)
			}
			addrs = append(addrs, ma)
			continue
		}

		if strings.HasPrefix(line, "/") {
			ma, err := multiaddr.NewMultiaddr(line)
			if err != nil {
				return nil, fmt.Errorf("parse bootnode multiaddr %q: %w", line, err)
			}
			addrs = append(addrs, ma)
			continue
		}

		return nil, fmt.Errorf("invalid bootnode line %d: %q", lineNo, line)
	}

	return addrs, scanner.Err()
}
