// Package textdiff provides a small unified-diff implementation for skill/profile compare.
package textdiff

import (
	"fmt"
	"strings"
)

// Unified returns a unified diff of a → b with the given path labels.
// When the texts are equal, returns an empty string.
func Unified(aLabel, bLabel, a, b string) string {
	if a == b {
		return ""
	}
	aLines := splitLines(a)
	bLines := splitLines(b)
	ops := myers(aLines, bLines)

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n", aLabel)
	fmt.Fprintf(&out, "+++ %s\n", bLabel)

	// Emit a single hunk covering the whole file (skills are small).
	// Count context-style ranges for a readable header.
	aCount, bCount := len(aLines), len(bLines)
	if aCount == 0 {
		aCount = 0
	}
	if bCount == 0 {
		bCount = 0
	}
	fmt.Fprintf(&out, "@@ -1,%d +1,%d @@\n", max(1, len(aLines)), max(1, len(bLines)))
	_ = aCount
	_ = bCount

	for _, op := range ops {
		switch op.Kind {
		case opEqual:
			for _, line := range op.Lines {
				fmt.Fprintf(&out, " %s\n", line)
			}
		case opDelete:
			for _, line := range op.Lines {
				fmt.Fprintf(&out, "-%s\n", line)
			}
		case opInsert:
			for _, line := range op.Lines {
				fmt.Fprintf(&out, "+%s\n", line)
			}
		}
	}
	return out.String()
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type op struct {
	Kind  opKind
	Lines []string
}

// myers is a compact LCS-based edit script (fine for skill-sized bodies).
func myers(a, b []string) []op {
	n, m := len(a), len(b)
	// DP LCS lengths
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var raw []op
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			raw = append(raw, op{Kind: opEqual, Lines: []string{a[i]}})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			raw = append(raw, op{Kind: opDelete, Lines: []string{a[i]}})
			i++
		} else {
			raw = append(raw, op{Kind: opInsert, Lines: []string{b[j]}})
			j++
		}
	}
	for i < n {
		raw = append(raw, op{Kind: opDelete, Lines: []string{a[i]}})
		i++
	}
	for j < m {
		raw = append(raw, op{Kind: opInsert, Lines: []string{b[j]}})
		j++
	}
	return coalesce(raw)
}

func coalesce(in []op) []op {
	if len(in) == 0 {
		return nil
	}
	out := []op{in[0]}
	for _, o := range in[1:] {
		last := &out[len(out)-1]
		if last.Kind == o.Kind {
			last.Lines = append(last.Lines, o.Lines...)
		} else {
			out = append(out, o)
		}
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Preserve trailing empty line semantics like diff tools.
	return strings.Split(s, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
