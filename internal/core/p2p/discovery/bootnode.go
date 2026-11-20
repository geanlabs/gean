package discovery

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/multiformats/go-multiaddr"
)

// LoadBootnodes reads a bootnode file: one ENR or multiaddr per line or YAML
// list entry, with # comments.
func LoadBootnodes(path string) ([]multiaddr.Multiaddr, error) {
	entries, err := readBootnodeEntries(path)
	if err != nil {
		return nil, err
	}
	var addrs []multiaddr.Multiaddr
	for _, e := range entries {
		if strings.HasPrefix(e.text, "enr:") {
			ma, err := ParseENR(e.text)
			if err != nil {
				return nil, fmt.Errorf("parse ENR bootnode: %w", err)
			}
			addrs = append(addrs, ma)
			continue
		}
		if strings.HasPrefix(e.text, "/") {
			ma, err := multiaddr.NewMultiaddr(e.text)
			if err != nil {
				return nil, fmt.Errorf("parse bootnode multiaddr %q: %w", e.text, err)
			}
			addrs = append(addrs, ma)
			continue
		}
		return nil, fmt.Errorf("invalid bootnode line %d: %q", e.line, e.text)
	}
	return addrs, nil
}

// LoadENRs returns the ENR entries of a bootnode file, for discv5 to start
// from. Multiaddr entries are skipped: discv5 cannot use them.
func LoadENRs(path string) ([]*enode.Node, error) {
	entries, err := readBootnodeEntries(path)
	if err != nil {
		return nil, err
	}
	var nodes []*enode.Node
	for _, e := range entries {
		if !strings.HasPrefix(e.text, "enr:") {
			continue
		}
		n, err := enode.Parse(enode.ValidSchemes, e.text)
		if err != nil {
			return nil, fmt.Errorf("parse ENR bootnode on line %d: %w", e.line, err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

type bootnodeEntry struct {
	line int
	text string
}

func readBootnodeEntries(path string) ([]bootnodeEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open bootnodes file: %w", err)
	}
	defer f.Close()

	var entries []bootnodeEntry
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
		entries = append(entries, bootnodeEntry{line: lineNo, text: line})
	}
	return entries, scanner.Err()
}
