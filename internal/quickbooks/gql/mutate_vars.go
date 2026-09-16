package gql

import "regexp"

// varDeclNameRe locates each variable declaration marker "$name:" in a
// signature. Type text is recovered by slicing between consecutive markers
// (see MutationVarTypes): Go's regexp has no lookahead, and webpack-extracted
// signatures separate declarations with arbitrary whitespace.
var varDeclNameRe = regexp.MustCompile(`\$([A-Za-z_][A-Za-z_0-9]*)\s*:`)
