package docsgen

import (
	"fmt"
	"regexp"
	"strings"
)

// ledgerObject is one CREATE TABLE or CREATE VIEW in spec/ledger.sql, with
// the comment block written directly above it and any index on it.
type ledgerObject struct {
	Name       string
	Kind       string // table | view
	Definition string // comment block + statement + its indexes, verbatim
	Columns    []column
	Constraint []string // table-level constraints (UNIQUE(...), PRIMARY KEY (...))
}

type column struct {
	Name       string
	Type       string
	Constraint string
}

var (
	createRe   = regexp.MustCompile(`^CREATE (TABLE|VIEW|INDEX) (\w+)`)
	indexOnRe  = regexp.MustCompile(`^CREATE INDEX \w+ ON (\w+)\(`)
	columnRe   = regexp.MustCompile(`^\s*(\w+)\s+(TEXT|REAL|INTEGER|BLOB)\b\s*([^,]*?)\s*,?\s*(?:--.*)?$`)
	tableConRe = regexp.MustCompile(`^\s*((?:UNIQUE|PRIMARY KEY|FOREIGN KEY)\s*\(.*?\))\s*,?\s*(?:--.*)?$`)
)

// parseLedgerSQL walks the file line by line: a run of `--` lines directly
// above a CREATE attaches to it; a blank line between them makes the
// comment a section note, which is dropped. CREATE INDEX attaches to its
// table. Column lines inside a CREATE TABLE are parsed for the columns table.
func parseLedgerSQL(sql string) ([]ledgerObject, error) {
	lines := strings.Split(sql, "\n")
	var objects []ledgerObject
	byName := map[string]int{}
	var comment []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "--"):
			comment = append(comment, line)
			continue
		case strings.TrimSpace(line) == "":
			comment = nil
			continue
		}
		m := createRe.FindStringSubmatch(line)
		if m == nil {
			comment = nil
			continue
		}
		// Collect the statement through its terminating semicolon.
		var stmt []string
		for ; i < len(lines); i++ {
			stmt = append(stmt, lines[i])
			if sql, _, _ := strings.Cut(lines[i], "--"); strings.Contains(sql, ";") {
				break
			}
		}
		text := strings.Join(append(append([]string{}, comment...), stmt...), "\n")
		comment = nil
		kind, name := strings.ToLower(m[1]), m[2]
		if kind == "index" {
			im := indexOnRe.FindStringSubmatch(line)
			if im == nil {
				return nil, fmt.Errorf("index %s: cannot tell its table", name)
			}
			idx, ok := byName[im[1]]
			if !ok {
				return nil, fmt.Errorf("index %s on unknown table %s", name, im[1])
			}
			objects[idx].Definition += "\n" + strings.Join(stmt, "\n")
			continue
		}
		obj := ledgerObject{Name: name, Kind: kind, Definition: text}
		if kind == "table" {
			for _, l := range stmt[1:] {
				if cm := columnRe.FindStringSubmatch(l); cm != nil {
					obj.Columns = append(obj.Columns, column{Name: cm[1], Type: cm[2], Constraint: strings.TrimSpace(cm[3])})
				} else if tm := tableConRe.FindStringSubmatch(l); tm != nil {
					obj.Constraint = append(obj.Constraint, tm[1])
				}
			}
			if len(obj.Columns) == 0 {
				return nil, fmt.Errorf("table %s: no columns parsed", name)
			}
		}
		byName[name] = len(objects)
		objects = append(objects, obj)
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("no CREATE statements")
	}
	return objects, nil
}
