package lsp

import (
	"context"
	"fmt"
	"time"
)

// MarksmanClient implements Client for Markdown files using a marksman
// subprocess in stdio mode.
type MarksmanClient struct {
	*stdioClient
}

// NewMarksmanClient starts marksman rooted at rootDir.
func NewMarksmanClient(rootDir string, timeout time.Duration) (*MarksmanClient, error) {
	c, err := newStdioClient("marksman", []string{"server"}, map[string]int{
		".md":       100,
		".markdown": 95,
	}, "markdown", rootDir, timeout)
	if err != nil {
		return nil, fmt.Errorf("marksman: %w", err)
	}
	return &MarksmanClient{stdioClient: c}, nil
}

// Imports implements Client for Markdown files.
// Markdown does not define language imports in a way compatible with codegraph
// import edges, so this returns an empty set.
func (c *MarksmanClient) Imports(_ context.Context, _ string) ([]Import, error) {
	return nil, nil
}
