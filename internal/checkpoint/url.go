package checkpoint

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	StatesFinalizedPath = "/lean/v0/states/finalized"
	BlocksFinalizedPath = "/lean/v0/blocks/finalized"
)

func deriveBlockURL(stateURL string) (string, error) {
	parsed, err := url.Parse(stateURL)
	if err != nil {
		return "", fmt.Errorf("parse checkpoint URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("checkpoint URL must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("checkpoint URL must include a host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("checkpoint URL must not include user info")
	}
	if !strings.HasSuffix(parsed.Path, StatesFinalizedPath) {
		return "", fmt.Errorf(
			"checkpoint URL %q must end with %q",
			stateURL, StatesFinalizedPath,
		)
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, StatesFinalizedPath) + BlocksFinalizedPath
	return parsed.String(), nil
}
