package rewrite

// LineRewriter lets you copy/cut/paste at the granularity of whole lines.
type LineRewriter interface {
	// CopyLinesUntil writes original lines [0..lineIndex-1], positioning the scanner at lineIndex.
	CopyLinesUntil(lineIndex int) error

	// ReplaceLines replaces all original lines from startLine through endLine (inclusive)
	// with the slice of newLines (each element is one line, without a trailing '\n').
	//
	// Internally, this means:
	//   1. Copy any lines < startLine
	//   2. Consume (skip) original lines [startLine..endLine]
	//   3. Insert each line from newLines (appending '\n' to each)
	//   4. Leave scanner positioned at line endLine+1, ready for further Copy/Insert/Replace calls
	ReplaceLines(startLine, endLine int, newLines [][]byte) error

	// CopyRemainingLines writes all leftover original lines (from current scanner position to EOF).
	CopyRemainingLines() error

	// LineIndexOfByte maps a byte-offset in the original content to its 0-based line index.
	LineIndexOfByte(offset int) int

	// Bytes returns the fully rewritten buffer.
	Bytes() []byte
}
