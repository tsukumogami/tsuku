package main_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// AC46 and AC50. Two searches over the span the design records, in both
// directions: no site the searches find is absent from the recorded list, and
// no listed site is absent from the code.
//
// AC50 is the part that decides the shape. The expected sites are read out of
// the recorded derivation -- the four in-scope functions out of the rule, the
// two boundaries out of the sentence naming them, the sites out of the table
// of derived rows -- rather than out of a list kept beside this check. A list
// here would go stale the way the two enumerations before it did, and it would
// go stale silently, because nothing would be comparing it to anything. What
// fails instead is the record, which is the artifact three requirements and
// four criteria quantify over.
//
// The searches are AC46's: reads and writes of the effective mode, and returns
// that reach an install or an exec before the dispatch. Neither is a match on
// row names. The first finds the mode's own constants and the functions that
// return one; the second computes what "reaches an install or an exec" means
// from the package rather than being told. So a site added to the code without
// a row appears as a find nothing claims, and a row whose site was deleted
// appears as a row nothing found.

const derivationRecordPath = "docs/designs/DESIGN-autoinstall-mode-resolution.md"

// The record's landmarks. Each is matched as a line prefix, which is specific
// enough that renaming one fails loudly rather than matching something else
// further down -- the direction the design says it wants.
const (
	derivationSectionHeading = "## The Gates Table, Re-derived"
	derivedRowsHeading       = "**The derived rows after this work**"
	spanBoundariesHeading    = "**The span's two boundaries, by identifier:**"
)

// autoinstallPackageDir is scanned to work out which functions reach an
// install or an exec. It is not where the sites come from -- those come from
// the record -- but the span's rule is stated in terms of installing and
// execing, and this check has to be able to recognize one.
const autoinstallPackageDir = "internal/autoinstall"

var backtickedText = regexp.MustCompile("`([^`]+)`")

// modeConstant matches the effective mode's own values. Reading one is the
// clearest form of the first limb: a point that mentions ModeConfirm is
// reading the mode whatever else it does.
var modeConstant = regexp.MustCompile(`^Mode[A-Z]`)

// A recordedSite is one row of the derived-rows table.
type recordedSite struct {
	Row      string
	File     string
	Function string
	Anchor   string
	Role     string
}

// wholeFunction reports whether the row is a function rather than a point
// inside one. Those rows absorb every mode read and write in their body, which
// is the anchoring the record states: a row sits at the function that decides
// the value, not at every line that touches it.
func (s recordedSite) wholeFunction() bool { return s.Anchor == s.Function }

func (s recordedSite) String() string {
	return fmt.Sprintf("%s (%s, %s, anchor %s)", s.Row, s.File, s.Function, s.Anchor)
}

// A foundSite is one point the searches produced.
type foundSite struct {
	Function string // the in-scope function it was found in
	Anchor   string // the identifier that makes it qualify
	Pos      string // file:line
	Search   int    // 1 for a mode read or write, 2 for a return that installs or execs
}

func (f foundSite) String() string {
	return fmt.Sprintf("%s in %s at %s (search %d)", f.Anchor, f.Function, f.Pos, f.Search)
}

// derivationRecord is what the check knows, all of it parsed out of the design.
type derivationRecord struct {
	InScope    []string // functions the rule names, receiver stripped
	StartCall  string   // the call that opens the span, as the record writes it
	StartFunc  string   // the function it sits in
	EndFunc    string   // the function the dispatch sits in
	Sites      []recordedSite
	SourceFile string // the one file the rows name
}

func TestDerivationMatchesTheCode(t *testing.T) {
	record, err := readDerivationRecord()
	if err != nil {
		t.Fatalf("reading the recorded derivation: %v\n\n"+
			"This check compares %s against the code the derivation describes. It cannot pass by "+
			"finding nothing: a record it cannot read is a record nobody is checking.",
			err, derivationRecordPath)
	}

	found, err := searchSpan(record)
	if err != nil {
		t.Fatalf("searching the span the record names: %v\n\n"+
			"The span is %s in %s through the mode dispatch in %s, and the record is %s.",
			err, record.StartCall, record.StartFunc, record.EndFunc, derivationRecordPath)
	}
	if len(found) == 0 {
		t.Fatalf("the searches found nothing in %s over the span %s..%s.\n\n"+
			"Both directions of this comparison are trivially satisfied by an empty search, so "+
			"finding nothing fails rather than passes. Either the span collapsed or the searches "+
			"stopped recognizing what they look for.",
			record.SourceFile, record.StartFunc, record.EndFunc)
	}

	claimed := make(map[recordedSite][]foundSite, len(record.Sites))
	var unclaimed []foundSite
	for _, f := range found {
		site, ok := attributeToRow(f, record.Sites)
		if !ok {
			unclaimed = append(unclaimed, f)
			continue
		}
		claimed[site] = append(claimed[site], f)
	}

	for _, f := range unclaimed {
		t.Errorf("the searches found a site in the span that no row of the recorded derivation "+
			"claims: %s.\n"+
			"A point reading or writing the effective mode, or returning into an install or an "+
			"exec, is a row by the rule the record states. Add it to the derived-rows table in %s "+
			"with an anchor naming the identifier it turns on.",
			f, derivationRecordPath)
	}

	for _, site := range record.Sites {
		if len(claimed[site]) > 0 {
			continue
		}
		t.Errorf("the recorded derivation lists a site the searches did not find: %s.\n"+
			"Either the site moved or was deleted and the row stayed behind, or the anchor no "+
			"longer names what the site turns on. A row whose site was deleted otherwise passes "+
			"forever, which is why this direction is checked too.",
			site)
	}
}

// TestDerivationRecordsOneSiteList closes AC52 in the only way the record
// allows, and says plainly why that is less than it sounds.
//
// AC52 compares the recorded derivation's site list against the derived rows of
// the table recorded beside it. The design records both as one table, and says
// so where it says the list is in one place so that AC46, AC50 and AC52 point
// at the same thing. The two sets are therefore equal by identity rather than
// by agreement, and a check comparing them would be comparing a table to
// itself.
//
// The honest guard is over the premise instead. If a second derived-rows table
// ever appears in this document the identity stops holding, the two lists can
// drift, and AC52 becomes a comparison somebody has to actually make. This
// fails at that moment rather than after the drift.
//
// Manufacturing a second table now, so there would be something to compare,
// would be writing the stale copy this whole document exists to prevent.
func TestDerivationRecordsOneSiteList(t *testing.T) {
	section, err := derivationSection()
	if err != nil {
		t.Fatalf("reading %s: %v", derivationRecordPath, err)
	}

	headings := 0
	for _, line := range section {
		if strings.HasPrefix(strings.TrimSpace(line), derivedRowsHeading) {
			headings++
		}
	}
	switch headings {
	case 1:
		return
	case 0:
		t.Fatalf("no line in %q begins %q, so there is no derived-rows table to compare anything "+
			"against.", derivationSectionHeading, derivedRowsHeading)
	default:
		t.Errorf("%q holds %d derived-rows tables. AC52 holds by identity only while there is one: "+
			"the derivation's site list and the derived rows are the same table. With two they can "+
			"disagree, and the comparison AC52 asks for has to be built rather than assumed.",
			derivationSectionHeading, headings)
	}
}

// derivationSection returns the lines of the recorded derivation, from its
// heading to the next section.
func derivationSection() ([]string, error) {
	body, err := os.ReadFile(derivationRecordPath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(body), "\n")

	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == derivationSectionHeading {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("no line reads %q; the section was renamed or removed", derivationSectionHeading)
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			return lines[start:i], nil
		}
	}
	return lines[start:], nil
}

// readDerivationRecord parses the whole of what this check knows out of the
// recorded derivation. Every way of finding nothing is an error rather than an
// empty result, because an empty result would make the comparison above pass.
func readDerivationRecord() (derivationRecord, error) {
	var record derivationRecord

	section, err := derivationSection()
	if err != nil {
		return record, err
	}

	if record.InScope, err = ruleFunctions(section); err != nil {
		return record, err
	}
	if record.StartCall, record.StartFunc, record.EndFunc, err = spanBoundaries(section); err != nil {
		return record, err
	}
	if record.Sites, err = derivedRows(section); err != nil {
		return record, err
	}

	files := map[string]bool{}
	for _, site := range record.Sites {
		files[site.File] = true
	}
	if len(files) != 1 {
		return record, fmt.Errorf("the derived rows name %d files: %v. This check parses one file, "+
			"so a derivation that spans more than one has outgrown it", len(files), sortedKeys(files))
	}
	for f := range files {
		record.SourceFile = f
	}

	for _, site := range record.Sites {
		if !contains(record.InScope, site.Function) {
			return record, fmt.Errorf("the derived rows put %q in %s, which the rule does not name "+
				"as in scope (%v). One of the two is wrong", site.Row, site.Function, record.InScope)
		}
	}
	if !contains(record.InScope, record.StartFunc) || !contains(record.InScope, record.EndFunc) {
		return record, fmt.Errorf("the span runs %s..%s but the rule names %v as in scope; the "+
			"boundaries have to be inside the span they bound",
			record.StartFunc, record.EndFunc, record.InScope)
	}
	return record, nil
}

// ruleFunctions reads the in-scope functions out of the rule, which the record
// states as the section's first block quote. Receivers are stripped: the record
// writes Runner.Run and the file declares Run.
func ruleFunctions(section []string) ([]string, error) {
	var quoted []string
	inQuote := false
	for _, line := range section {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">") {
			inQuote = true
			quoted = append(quoted, trimmed)
			continue
		}
		if inQuote && trimmed == "" {
			break
		}
	}
	if len(quoted) == 0 {
		return nil, fmt.Errorf("%q holds no block quote; the rule the span is derived against is "+
			"stated as one", derivationSectionHeading)
	}

	var funcs []string
	for _, match := range backtickedText.FindAllStringSubmatch(strings.Join(quoted, " "), -1) {
		funcs = append(funcs, baseName(match[1]))
	}
	if len(funcs) == 0 {
		return nil, fmt.Errorf("the rule names no functions in backticks; it has to say where it looks")
	}
	return funcs, nil
}

// spanBoundaries reads the two ends of the span out of the sentence that names
// them. The record writes three identifiers there, in order: the call that
// opens the span, the function holding it, and the function holding the
// dispatch that closes it.
func spanBoundaries(section []string) (startCall, startFunc, endFunc string, err error) {
	sentence, ok := paragraphStarting(section, spanBoundariesHeading)
	if !ok {
		return "", "", "", fmt.Errorf("no line begins %q; the record has to say where it looked, "+
			"not only what it found", spanBoundariesHeading)
	}

	matches := backtickedText.FindAllStringSubmatch(sentence, -1)
	if len(matches) != 3 {
		return "", "", "", fmt.Errorf("the boundaries sentence names %d identifiers in backticks, "+
			"want 3: the opening call, the function holding it, and the function holding the "+
			"dispatch", len(matches))
	}
	return matches[0][1], baseName(matches[1][1]), baseName(matches[2][1]), nil
}

// derivedRows reads the table of derived rows, taking columns by their heading
// rather than by position so that reordering them is not a silent change.
func derivedRows(section []string) ([]recordedSite, error) {
	rows, err := markdownTableAfter(section, derivedRowsHeading)
	if err != nil {
		return nil, err
	}

	var sites []recordedSite
	for _, row := range rows {
		site := recordedSite{
			Row:      row["row"],
			File:     backticked(row["file"]),
			Function: baseName(backticked(row["function"])),
			Anchor:   baseName(backticked(row["anchor"])),
			Role:     row["role"],
		}
		if site.Row == "" || site.File == "" || site.Function == "" || site.Anchor == "" || site.Role == "" {
			return nil, fmt.Errorf("a derived row is missing a column: %+v. AC45 wants file, "+
				"function and role, and the anchor is what makes a row findable when three of "+
				"them share a function", row)
		}
		sites = append(sites, site)
	}

	seen := map[string]string{}
	for _, site := range sites {
		key := site.Function + "." + site.Anchor
		if first, ok := seen[key]; ok {
			return nil, fmt.Errorf("%q and %q are both anchored at %s, so neither can be found "+
				"separately", first, site.Row, key)
		}
		seen[key] = site.Row
	}
	return sites, nil
}

// markdownTableAfter returns the rows of the first table following a heading,
// keyed by lowercased column name.
func markdownTableAfter(section []string, heading string) ([]map[string]string, error) {
	start := -1
	for i, line := range section {
		if strings.HasPrefix(strings.TrimSpace(line), heading) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("no line begins %q; the section was renamed or the table removed", heading)
	}

	var table []string
	inTable := false
	for _, line := range section[start+1:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if inTable {
				break
			}
			continue
		}
		inTable = true
		table = append(table, trimmed)
	}
	if len(table) < 3 {
		return nil, fmt.Errorf("the table after %q has %d lines; a heading, a separator and at "+
			"least one row are needed", heading, len(table))
	}

	headings := splitRow(table[0])
	for i, h := range headings {
		headings[i] = strings.ToLower(h)
	}

	var rows []map[string]string
	for _, line := range table[2:] {
		cells := splitRow(line)
		if len(cells) != len(headings) {
			return nil, fmt.Errorf("a row of the table after %q has %d cells against %d headings: %q",
				heading, len(cells), len(headings), line)
		}
		row := make(map[string]string, len(headings))
		for i, h := range headings {
			row[h] = cells[i]
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("the table after %q records no rows", heading)
	}
	return rows, nil
}

func splitRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	cells := strings.Split(trimmed, "|")
	for i, c := range cells {
		cells[i] = strings.TrimSpace(c)
	}
	return cells
}

// paragraphStarting returns the paragraph beginning with the given prefix,
// joined onto one line. It is joined because the record is wrapped prose and a
// sentence the formatter split across two lines is still one sentence.
func paragraphStarting(section []string, prefix string) (string, bool) {
	for i, line := range section {
		if !strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		var para []string
		for _, l := range section[i:] {
			if strings.TrimSpace(l) == "" {
				break
			}
			para = append(para, strings.TrimSpace(l))
		}
		return strings.Join(para, " "), true
	}
	return "", false
}

// backticked returns the first backticked run in a table cell, which is how
// the record writes an identifier. A cell with no backticks is returned as it
// stands, so a row that stops quoting its identifier fails somewhere with a
// name in the message rather than here with an empty one.
func backticked(cell string) string {
	if match := backtickedText.FindStringSubmatch(cell); match != nil {
		return match[1]
	}
	return cell
}

// baseName drops a receiver or a qualifier: Runner.Run is Run, r.Lookup is
// Lookup. The record writes identifiers the way prose wants them and the file
// declares them bare.
func baseName(identifier string) string {
	if i := strings.LastIndex(identifier, "."); i >= 0 {
		return identifier[i+1:]
	}
	return identifier
}

// searchSpan runs AC46's two searches over the span the record names.
func searchSpan(record derivationRecord) ([]foundSite, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, record.SourceFile, nil, 0)
	if err != nil {
		return nil, err
	}

	funcs := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			funcs[fn.Name.Name] = fn
		}
	}
	for _, name := range record.InScope {
		if funcs[name] == nil {
			return nil, fmt.Errorf("the rule names %s but %s declares no such function; the rule "+
				"and the code have parted company", name, record.SourceFile)
		}
	}

	// The span's opening: the call the record names, inside the function it
	// names. Everything before it in that function is outside the span.
	openPos, err := singleCall(fset, funcs[record.StartFunc], record.StartCall)
	if err != nil {
		return nil, fmt.Errorf("locating the span's opening: %w", err)
	}

	// The span's close: the mode dispatch. It is the one switch in the closing
	// function, and it is the boundary rather than a site -- a row for it would
	// be a row for the thing every other row is measured against.
	dispatchPos, err := singleSwitch(funcs[record.EndFunc])
	if err != nil {
		return nil, fmt.Errorf("locating the mode dispatch that closes the span: %w", err)
	}

	deciders := modeDecidingFunctions(file)
	if len(deciders) == 0 {
		return nil, fmt.Errorf("%s declares no function returning a Mode, so nothing in it decides "+
			"the effective mode and the first search has nothing to find", record.SourceFile)
	}
	terminal, err := installOrExecFunctions()
	if err != nil {
		return nil, err
	}

	wholeFunctionRows := map[string]bool{}
	for _, site := range record.Sites {
		if site.wholeFunction() {
			wholeFunctionRows[site.Function] = true
		}
	}

	var found []foundSite
	for _, name := range record.InScope {
		fn := funcs[name]
		inSpan := func(pos token.Pos) bool {
			if name == record.StartFunc && pos < openPos {
				return false
			}
			if name == record.EndFunc && pos >= dispatchPos {
				return false
			}
			return true
		}

		found = append(found, searchModeUses(fset, fn, name, inSpan, deciders)...)
		found = append(found, searchTerminalReturns(fset, fn, name, inSpan, terminal)...)

		// A read of the mode through a variable alone names no constant and
		// calls nothing, so the search above cannot anchor it. Left there it
		// would be a site the record never has to mention. Rows that are whole
		// functions are exempt: they claim everything in their body already.
		if !wholeFunctionRows[name] {
			found = append(found, searchBareModeReads(fset, fn, name, inSpan, deciders)...)
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].Pos < found[j].Pos })
	return found, nil
}

// searchModeUses is AC46's first search: reads and writes of the effective
// mode. It recognizes one by the mode's own constants and by calls to the
// functions that return a Mode, which are the two ways this file has of
// touching the value.
func searchModeUses(fset *token.FileSet, fn *ast.FuncDecl, name string, inSpan func(token.Pos) bool, deciders map[string]bool) []foundSite {
	var found []foundSite
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil || !inSpan(n.Pos()) {
			return true
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			if callee := calleeName(node); deciders[callee] {
				found = append(found, foundSite{
					Function: name, Anchor: callee, Pos: position(fset, node.Pos()), Search: 1,
				})
			}
		case *ast.Ident:
			if modeConstant.MatchString(node.Name) {
				found = append(found, foundSite{
					Function: name, Anchor: node.Name, Pos: position(fset, node.Pos()), Search: 1,
				})
			}
		}
		return true
	})
	return found
}

// searchBareModeReads finds statements that use a Mode-typed variable without
// naming a constant or calling a decider, which is a read the first search
// cannot anchor. There are none today. The check reports one as a site the
// record does not claim, because that is what it is.
func searchBareModeReads(fset *token.FileSet, fn *ast.FuncDecl, name string, inSpan func(token.Pos) bool, deciders map[string]bool) []foundSite {
	vars := modeVariables(fn, deciders)
	if len(vars) == 0 {
		return nil
	}
	parents := parentMap(fn.Body)

	var found []foundSite
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || !vars[ident.Name] || !inSpan(ident.Pos()) {
			return true
		}
		stmt := enclosingStmt(ident, parents)
		if stmt == nil || statementAnchors(stmt, deciders) {
			return true
		}
		found = append(found, foundSite{
			Function: name, Anchor: ident.Name, Pos: position(fset, ident.Pos()), Search: 1,
		})
		return true
	})
	return found
}

// searchTerminalReturns is AC46's second search: returns that reach an install
// or an exec before the dispatch. That predicate is what admits the fast path
// and excludes the root guard and the two error returns, which end the run
// without reaching either.
func searchTerminalReturns(fset *token.FileSet, fn *ast.FuncDecl, name string, inSpan func(token.Pos) bool, terminal map[string]bool) []foundSite {
	var found []foundSite
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || !inSpan(ret.Pos()) {
			return true
		}
		for _, result := range ret.Results {
			ast.Inspect(result, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				if callee := calleeName(call); terminal[callee] {
					found = append(found, foundSite{
						Function: name, Anchor: callee, Pos: position(fset, ret.Pos()), Search: 2,
					})
				}
				return true
			})
		}
		return true
	})
	return found
}

// attributeToRow decides which recorded row a find belongs to.
//
// The two indirect cases are the record's own anchoring rule. A find inside a
// function that is itself a row belongs to that row rather than being a row of
// its own, and a call to such a function from elsewhere is the same site as the
// function it calls -- Run's two assignments are the elevate and lowerMode
// rows, not two more.
func attributeToRow(f foundSite, sites []recordedSite) (recordedSite, bool) {
	for _, site := range sites {
		if site.wholeFunction() && site.Function == f.Function {
			return site, true
		}
	}
	for _, site := range sites {
		if site.wholeFunction() && site.Function == f.Anchor {
			return site, true
		}
	}
	for _, site := range sites {
		if site.Function == f.Function && site.Anchor == f.Anchor {
			return site, true
		}
	}
	return recordedSite{}, false
}

// modeDecidingFunctions returns the functions in the file that return a Mode.
// They are read off the declarations rather than listed, so a third one added
// later is searched for without this check being told about it.
func modeDecidingFunctions(file *ast.File) map[string]bool {
	deciders := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type.Results == nil {
			continue
		}
		for _, result := range fn.Type.Results.List {
			if ident, ok := result.Type.(*ast.Ident); ok && ident.Name == "Mode" {
				deciders[fn.Name.Name] = true
			}
		}
	}
	return deciders
}

// installOrExecFunctions returns the names a return can call to reach an
// install or an exec: the install and exec calls themselves, and the functions
// in the package whose bodies make one.
//
// It is computed rather than listed for the same reason as the deciders. A
// second exec helper added beside execBinary is recognized without an edit
// here, and a return through it is a site the record has to claim.
func installOrExecFunctions() (map[string]bool, error) {
	terminal := map[string]bool{"Exec": true, "Install": true}

	entries, err := os.ReadDir(autoinstallPackageDir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, filepath.Join(autoinstallPackageDir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, parsed)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s holds no source files, so nothing in it installs or execs and "+
			"the second search has nothing to find", autoinstallPackageDir)
	}

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && terminal[calleeName(call)] {
					terminal[fn.Name.Name] = true
				}
				return true
			})
		}
	}
	return terminal, nil
}

// modeVariables returns the Mode-typed names in a function: its parameters
// declared as Mode, and whatever a decider's first result is assigned to.
func modeVariables(fn *ast.FuncDecl, deciders map[string]bool) map[string]bool {
	vars := map[string]bool{}
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			ident, ok := param.Type.(*ast.Ident)
			if !ok || ident.Name != "Mode" {
				continue
			}
			for _, name := range param.Names {
				vars[name.Name] = true
			}
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !deciders[calleeName(call)] {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			vars[ident.Name] = true
		}
		return true
	})
	return vars
}

// statementAnchors reports whether a statement names something the first
// search can anchor a site to.
func statementAnchors(stmt ast.Stmt, deciders map[string]bool) bool {
	anchored := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if deciders[calleeName(node)] {
				anchored = true
			}
		case *ast.Ident:
			if modeConstant.MatchString(node.Name) {
				anchored = true
			}
		}
		return !anchored
	})
	return anchored
}

// singleCall locates the one call to the named function inside fn. More than
// one, or none, means the boundary the record names does not identify a point.
func singleCall(fset *token.FileSet, fn *ast.FuncDecl, call string) (token.Pos, error) {
	var positions []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && exprString(fset, c.Fun) == call {
			positions = append(positions, c.Pos())
		}
		return true
	})
	switch len(positions) {
	case 1:
		return positions[0], nil
	case 0:
		return 0, fmt.Errorf("%s holds no call to %s", fn.Name.Name, call)
	default:
		return 0, fmt.Errorf("%s calls %s %d times, so the record's boundary names no single point",
			fn.Name.Name, call, len(positions))
	}
}

// singleSwitch locates the one switch in fn, which is the mode dispatch.
func singleSwitch(fn *ast.FuncDecl) (token.Pos, error) {
	var positions []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if s, ok := n.(*ast.SwitchStmt); ok {
			positions = append(positions, s.Pos())
		}
		return true
	})
	switch len(positions) {
	case 1:
		return positions[0], nil
	case 0:
		return 0, fmt.Errorf("%s holds no switch, so the span has no closing boundary", fn.Name.Name)
	default:
		return 0, fmt.Errorf("%s holds %d switches, so which one closes the span is a guess",
			fn.Name.Name, len(positions))
	}
}

func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

func position(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d", p.Filename, p.Line)
}

// parentMap records each node's parent, so a find can be walked back up to the
// statement holding it.
func parentMap(root ast.Node) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}

func enclosingStmt(n ast.Node, parents map[ast.Node]ast.Node) ast.Stmt {
	for cur := n; cur != nil; cur = parents[cur] {
		if stmt, ok := cur.(ast.Stmt); ok {
			return stmt
		}
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
