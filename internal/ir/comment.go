package ir

// Comment formats, the values of DistilledComment.Format. Text never carries
// the comment delimiters; Format says which kind of comment it was.
const (
	CommentLine   = "line"   // "// x", "# x"
	CommentBlock  = "block"  // "/* x */"
	CommentDoc    = "doc"    // documentation attached to the next declaration
	CommentExport = "export" // JavaScript export marker, existing special case
)

// commentDocLegacy is the format PHP still uses for its /** */ docblocks.
const commentDocLegacy = "docblock"

// IsDoc reports whether the comment is documentation ("doc", or the legacy "docblock").
func (n *DistilledComment) IsDoc() bool {
	return n.Format == CommentDoc || n.Format == commentDocLegacy
}
