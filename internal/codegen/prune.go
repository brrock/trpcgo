package codegen

import (
	"regexp"
	"strings"
)

var (
	tsIdentifier  = regexp.MustCompile(`[A-Za-z_$][\w$]*`)
	tsDeclaration = regexp.MustCompile(`^(export\s+)?(?:declare\s+)?(?:async\s+)?(?:function\*?|const|let|var|type|interface|class|enum)\s+([A-Za-z_$][\w$]*)`)
)

// pruneUnusedDeclarations removes each module-private top-level declaration
// that no other live statement references, until none is left. Helpers are
// emitted for every capability a module might need; this keeps only those its
// schemas use, so the module compiles under noUnusedLocals.
//
// The generator writes every top-level statement from column 0 and indents
// its continuation lines, or starts them with a closing bracket. A comment
// directly above a statement belongs to it.
func pruneUnusedDeclarations(source string) string {
	lines := strings.SplitAfter(source, "\n")
	starts := []int{0}
	for i, line := range lines {
		if i == 0 || line == "" || !isTSIdentifierStart(line[0]) {
			continue
		}
		start := i
		for start > starts[len(starts)-1]+1 && isTSComment(lines[start-1]) {
			start--
		}
		starts = append(starts, start)
	}

	type statement struct {
		text, name string // name is empty for statements that are always kept
		refs       map[string]bool
		live       bool
	}
	statements := make([]statement, len(starts))
	for i, start := range starts {
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		s := statement{text: strings.Join(lines[start:end], ""), refs: map[string]bool{}, live: true}
		for _, ident := range tsIdentifier.FindAllString(s.text, -1) {
			s.refs[ident] = true
		}
		head := start
		for head < end-1 && isTSComment(lines[head]) {
			head++
		}
		if m := tsDeclaration.FindStringSubmatch(lines[head]); m != nil && m[1] == "" {
			s.name = m[2]
		}
		statements[i] = s
	}

	for changed := true; changed; {
		changed = false
		users := map[string]int{}
		for _, s := range statements {
			if s.live {
				for ident := range s.refs {
					users[ident]++
				}
			}
		}
		for i := range statements {
			s := &statements[i]
			if s.live && s.name != "" && users[s.name] == 1 { // only itself
				s.live, changed = false, true
			}
		}
	}

	var out strings.Builder
	for _, s := range statements {
		if s.live {
			out.WriteString(s.text)
		}
	}
	return out.String()
}

func isTSIdentifierStart(c byte) bool {
	return c == '$' || c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func isTSComment(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*")
}
