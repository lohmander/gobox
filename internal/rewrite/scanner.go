package rewrite

import (
	"bufio"
	"bytes"
	"io"
	"sort"
)

// ScannerRewriter implements LineRewriter using bufio.Scanner.
type ScannerRewriter struct {
	scanner     *bufio.Scanner
	output      bytes.Buffer
	lineNo      int   // how many lines have been consumed (scanned) so far
	finished    bool  // true once we've reached EOF
	lineOffsets []int // precomputed byte-offset where each line begins
}

// NewScannerRewriter constructs a ScannerRewriter over an io.Reader (the full file content),
// plus a slice of line-start offsets (from BuildLineOffsets).
func NewScannerRewriter(r io.Reader, lineOffsets []int) *ScannerRewriter {
	return &ScannerRewriter{
		scanner:     bufio.NewScanner(r),
		lineOffsets: lineOffsets,
		lineNo:      0,
		finished:    false,
	}
}

// CopyLinesUntil writes original lines [0..lineIndex-1] to output and positions the scanner at lineIndex.
func (rw *ScannerRewriter) CopyLinesUntil(lineIndex int) error {
	if rw.finished {
		return nil
	}
	for rw.lineNo < lineIndex {
		if !rw.scanner.Scan() {
			rw.finished = true
			return rw.scanner.Err()
		}
		rw.output.Write(rw.scanner.Bytes())
		rw.output.WriteByte('\n')
		rw.lineNo++
	}
	return rw.scanner.Err()
}

// ReplaceLines replaces all original lines from startLine..endLine (inclusive) with newLines.
// newLines is a slice of byte slices, each representing one line (no trailing '\n').
func (rw *ScannerRewriter) ReplaceLines(startLine, endLine int, newLines [][]byte) error {
	// 1) Copy up to startLine (this consumes lines 0..startLine-1).
	if err := rw.CopyLinesUntil(startLine); err != nil {
		return err
	}
	// 2) Skip (consume without writing) lines [startLine..endLine]:
	linesToSkip := endLine - startLine + 1
	for i := 0; i < linesToSkip; i++ {
		if !rw.scanner.Scan() {
			rw.finished = true
			return rw.scanner.Err()
		}
		rw.lineNo++
	}
	// 3) Write each newLine + "\n"
	for _, nl := range newLines {
		rw.output.Write(nl)
		rw.output.WriteByte('\n')
	}
	return nil
}

// CopyRemainingLines writes all lines from the current scanner position through EOF.
func (rw *ScannerRewriter) CopyRemainingLines() error {
	if rw.finished {
		return nil
	}
	for rw.scanner.Scan() {
		rw.output.Write(rw.scanner.Bytes())
		rw.output.WriteByte('\n')
		rw.lineNo++
	}
	rw.finished = true
	return rw.scanner.Err()
}

// LineIndexOfByte returns the 0-based line index that contains offset (byte index in the original file).
func (rw *ScannerRewriter) LineIndexOfByte(offset int) int {
	i := sort.Search(len(rw.lineOffsets), func(i int) bool {
		return rw.lineOffsets[i] > offset
	})
	if i == 0 {
		return 0
	}
	return i - 1
}

// Bytes returns the fully rewritten buffer.
func (rw *ScannerRewriter) Bytes() []byte {
	return rw.output.Bytes()
}

// BuildLineOffsets returns a slice of byte offsets where each new line begins.
// E.g. if content[0]=='a' and content[5]=='\n', then offsets = [0,6,...].
func BuildLineOffsets(content []byte) []int {
	offsets := []int{0}
	for i, b := range content {
		if b == '\n' && i+1 < len(content) {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}
