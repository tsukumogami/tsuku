package main_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
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

// The record's landmarks. Each is matched as a line prefix, which is specific
// enough that renaming one fails loudly rather than matching something else
// further down -- the direction the design says it wants.
const (
	derivationSectionHeading = "## The Gates Table, Re-derived"
	derivedRowsHeading       = "**The derived rows after this work**"
	spanBoundariesHeading    = "**The span's two boundaries, by identifier:**"
)

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
	Points   int
	Role     string
}

// wholeFunction reports whether the row is a function rather than a point
// inside one. Those rows absorb every mode read and write in their body, which
// is the anchoring the record states: a row sits at the function that decides
// the value, not at every line that touches it.
func (s recordedSite) wholeFunction() bool { return s.Anchor == s.Function }

func (s recordedSite) String() string {
	return fmt.Sprintf("%s (%s, %s, anchor %s, %d point(s))", s.Row, s.File, s.Function, s.Anchor, s.Points)
}

// Which of AC46's two searches produced a find.
const (
	searchModeUse = iota + 1
	searchTerminalReturn
)

// A foundSite is one point the searches produced.
type foundSite struct {
	Function string // the in-scope function it was found in
	Anchor   string // the identifier that makes it qualify
	Pos      string // file:line
	Search   int    // which search found it
}

func (f foundSite) String() string {
	kind := "a mode read or write"
	if f.Search == searchTerminalReturn {
		kind = "a return that installs or execs"
	}
	return fmt.Sprintf("%s in %s at %s (%s)", f.Anchor, f.Function, f.Pos, kind)
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
			err, designRecordPath)
	}

	found, err := searchSpan(record)
	if err != nil {
		t.Fatalf("searching the span the record names: %v\n\n"+
			"The span is %s in %s through the mode dispatch in %s, and the record is %s.",
			err, record.StartCall, record.StartFunc, record.EndFunc, designRecordPath)
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
		site, absorbed, ok := attributeToRow(f, record.Sites)
		if !ok {
			unclaimed = append(unclaimed, f)
			continue
		}
		if absorbed {
			// A find inside a function that is itself a row. The record puts
			// the row at the function, so what its body does is that one site
			// however many lines it takes, and counting those lines would make
			// an edit inside elevate look like a new site.
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
			f, designRecordPath)
	}

	for _, site := range record.Sites {
		switch found := claimed[site]; {
		case len(found) == site.Points:
		case len(found) == 0:
			t.Errorf("the recorded derivation lists a site the searches did not find: %s -- %q.\n"+
				"Either the site moved or was deleted and the row stayed behind, or the anchor no "+
				"longer names what the site turns on. A row whose site was deleted otherwise "+
				"passes forever, which is why this direction is checked too.",
				site, site.Role)
		default:
			// The count is what stops a new site hiding behind an anchor that
			// is already recorded. A second branch turning on ModeConfirm is a
			// site the record does not name, and without this it would be
			// attributed to the terminal check and disappear.
			t.Errorf("the recorded derivation puts %d point(s) at %s, and the searches found %d: "+
				"%v.\n"+
				"Points sharing an anchor are the one thing the anchor cannot tell apart, so the "+
				"record says how many there are. Either a point was added or removed here, or the "+
				"count was never right.",
				site.Points, site, len(found), found)
		}
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
		t.Fatalf("reading %s: %v", designRecordPath, err)
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
	body, err := os.ReadFile(designRecordPath)
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
		if !slices.Contains(record.InScope, site.Function) {
			return record, fmt.Errorf("the derived rows put %q in %s, which the rule does not name "+
				"as in scope (%v). One of the two is wrong", site.Row, site.Function, record.InScope)
		}
	}
	if !slices.Contains(record.InScope, record.StartFunc) || !slices.Contains(record.InScope, record.EndFunc) {
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
	return baseName(matches[0][1]), baseName(matches[1][1]), baseName(matches[2][1]), nil
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
			return nil, fmt.Errorf("a derived row is missing a column: %+v. The columns are read by "+
				"their headings, so a renamed heading arrives here as an empty cell. Row, File, "+
				"Function and Role are what the criterion asks a row to cite; Anchor is what makes "+
				"a row findable when three of them share a function", row)
		}
		points, err := strconv.Atoi(row["points"])
		if err != nil || points < 1 {
			return nil, fmt.Errorf("the row %q records %q points, which is not a count of one or "+
				"more, in %+v. A row has to say how many points share its anchor, because the "+
				"anchor cannot tell them apart -- and an empty value here usually means the Points "+
				"heading was renamed rather than the cell emptied", site.Row, row["points"], row)
		}
		site.Points = points
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

	// The whole package, not just the recorded file. Both sets computed below
	// -- what decides a mode, what reaches an install or an exec -- are
	// properties of the package, and reading them out of one file would make a
	// helper moved next door invisible rather than loud.
	pkg, err := parsePackage(fset, filepath.Dir(record.SourceFile))
	if err != nil {
		return nil, err
	}
	file := pkg[record.SourceFile]
	if file == nil {
		return nil, fmt.Errorf("the rows name %s, which is not a source file of %s",
			record.SourceFile, filepath.Dir(record.SourceFile))
	}

	funcs := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			funcs[fn.Name.Name] = fn
		}
	}
	for _, name := range record.InScope {
		if funcs[name] == nil {
			return nil, fmt.Errorf("the rule names %s but %s declares no such function. Every "+
				"identifier the rule writes in backticks is read as one, so a rule that quotes a "+
				"type or a table reaches here too", name, record.SourceFile)
		}
	}

	deciders := modeDecidingFunctions(pkg)
	if len(deciders) == 0 {
		return nil, fmt.Errorf("%s declares no function returning a Mode, so nothing in it decides "+
			"the effective mode and the first search has nothing to find",
			filepath.Dir(record.SourceFile))
	}
	terminal := installOrExecFunctions(pkg)

	// The span's opening: the call the record names, inside the function it
	// names. Everything before it in that function is outside the span.
	openPos, err := boundaryCall(funcs[record.StartFunc], record.StartCall)
	if err != nil {
		return nil, fmt.Errorf("locating the span's opening: %w", err)
	}

	// The span's close: the mode dispatch, which is the switch on the mode
	// rather than whichever switch happens to be the only one. It is the
	// boundary rather than a site -- a row for it would be a row for the thing
	// every other row is measured against.
	dispatchPos, err := modeDispatch(funcs[record.EndFunc], deciders)
	if err != nil {
		return nil, fmt.Errorf("locating the mode dispatch that closes the span: %w", err)
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
	}

	found = dedupe(found)
	sort.Slice(found, func(i, j int) bool { return found[i].Pos < found[j].Pos })
	return found, nil
}

// dedupe collapses a point that both searches reach -- a return whose call
// installs or execs and is also handed the mode. Nothing in the code does that
// today. It is here because the count in the record is per point, so a point
// counted twice would fail as loudly as a point added, and for the wrong
// reason.
func dedupe(found []foundSite) []foundSite {
	seen := map[foundSite]bool{}
	out := make([]foundSite, 0, len(found))
	for _, f := range found {
		key := f
		key.Search = 0
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

// searchModeUses is AC46's first search: reads and writes of the effective
// mode. It recognizes one three ways -- the mode's own constants, a call to a
// function that returns a Mode, and a call handed the mode as an argument.
//
// The third is what keeps a future reader from being invisible. A site that
// passes the mode somewhere without comparing it to anything names no constant,
// and without this rule the record would never have to mention it.
func searchModeUses(fset *token.FileSet, fn *ast.FuncDecl, name string, inSpan func(token.Pos) bool, deciders map[string]bool) []foundSite {
	vars := modeVariables(fn, deciders)

	var found []foundSite
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil || !inSpan(n.Pos()) {
			return true
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			callee := calleeName(node)
			if deciders[callee] || passesMode(node, vars) {
				found = append(found, foundSite{
					Function: name, Anchor: callee, Pos: position(fset, node.Pos()), Search: searchModeUse,
				})
			}
		case *ast.Ident:
			if modeConstant.MatchString(node.Name) {
				found = append(found, foundSite{
					Function: name, Anchor: node.Name, Pos: position(fset, node.Pos()), Search: searchModeUse,
				})
			}
		}
		return true
	})
	return found
}

// passesMode reports whether a call is handed a Mode-typed variable directly.
func passesMode(call *ast.CallExpr, vars map[string]bool) bool {
	for _, arg := range call.Args {
		if ident, ok := arg.(*ast.Ident); ok && vars[ident.Name] {
			return true
		}
	}
	return false
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
						Function: name, Anchor: callee, Pos: position(fset, ret.Pos()), Search: searchTerminalReturn,
					})
				}
				return true
			})
		}
		return true
	})
	return found
}

// attributeToRow decides which recorded row a find belongs to, and reports
// whether the row absorbed it rather than counting it.
//
// The two indirect cases are the record's own anchoring rule. A find inside a
// function that is itself a row belongs to that row rather than being a row of
// its own -- absorbed -- and a call to such a function from elsewhere is the
// same site as the function it calls, which is the point the row counts. Run's
// two assignments are the elevate and lowerMode rows, not two more.
//
// The order matters and is not arbitrary: a whole-function row claims a call to
// it before any point row could, so a row written as Run/elevate would never be
// reached. That is the right way round -- the record says a row sits at the
// function that decides the value -- and it is why no such row exists.
func attributeToRow(f foundSite, sites []recordedSite) (recordedSite, bool, bool) {
	for _, site := range sites {
		if site.wholeFunction() && site.Function == f.Function {
			return site, true, true
		}
	}
	for _, site := range sites {
		if site.wholeFunction() && site.Function == f.Anchor {
			return site, false, true
		}
	}
	for _, site := range sites {
		if site.Function == f.Function && site.Anchor == f.Anchor {
			return site, false, true
		}
	}
	return recordedSite{}, false, false
}

// parsePackage parses every non-test source file in a directory, keyed by path.
func parsePackage(fset *token.FileSet, dir string) (map[string]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	pkg := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, err
		}
		pkg[path] = parsed
	}
	if len(pkg) == 0 {
		return nil, fmt.Errorf("%s holds no source files, so both searches have nothing to find", dir)
	}
	return pkg, nil
}

// modeDecidingFunctions returns the functions in the package that return a
// Mode. They are read off the declarations rather than listed, so a third one
// added later is searched for without this check being told about it.
func modeDecidingFunctions(pkg map[string]*ast.File) map[string]bool {
	deciders := map[string]bool{}
	forEachFunc(pkg, func(fn *ast.FuncDecl) {
		if fn.Type.Results == nil {
			return
		}
		for _, result := range fn.Type.Results.List {
			if ident, ok := result.Type.(*ast.Ident); ok && ident.Name == "Mode" {
				deciders[fn.Name.Name] = true
			}
		}
	})
	return deciders
}

// installOrExecFunctions returns the names a return can call to reach an
// install or an exec: the install and exec calls themselves, and the functions
// in the package that reach one through any number of hops.
//
// It is computed rather than listed for the same reason as the deciders. A
// second exec helper added beside execBinary is recognized without an edit
// here, and a return through it is a site the record has to claim.
//
// The loop runs to a fixed point rather than once. A single pass would make
// recognition depend on declaration order -- a helper written above the one it
// calls would not be terminal -- and the direction it fails in is the silent
// one: the return stops being a find, its row keeps its count, and nothing
// says so.
func installOrExecFunctions(pkg map[string]*ast.File) map[string]bool {
	terminal := map[string]bool{"Exec": true, "Install": true}
	for {
		grew := false
		forEachFunc(pkg, func(fn *ast.FuncDecl) {
			if fn.Body == nil || terminal[fn.Name.Name] {
				return
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && terminal[calleeName(call)] {
					terminal[fn.Name.Name] = true
					grew = true
				}
				return true
			})
		})
		if !grew {
			return terminal
		}
	}
}

func forEachFunc(pkg map[string]*ast.File, visit func(*ast.FuncDecl)) {
	for _, path := range sortedFileKeys(pkg) {
		for _, decl := range pkg[path].Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				visit(fn)
			}
		}
	}
}

func sortedFileKeys(pkg map[string]*ast.File) []string {
	paths := make([]string, 0, len(pkg))
	for path := range pkg {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
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

// boundaryCall locates the one call to the named function inside fn. More than
// one, or none, means the boundary the record names does not identify a point.
//
// The name is matched bare, so the record may write r.Lookup, Runner.Lookup or
// Lookup and mean the same call. Matching the receiver as written would make a
// document edit that normalizes those spellings fail with a message pointing at
// the code.
func boundaryCall(fn *ast.FuncDecl, call string) (token.Pos, error) {
	var positions []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && calleeName(c) == call {
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

// modeDispatch locates the switch on the mode, which is the span's close.
//
// It is the switch whose subject is the mode rather than the only switch in the
// function. Keying on "the only one" made an ordinary refactor above the
// dispatch -- turning an if-else chain into a switch -- fail as though the
// boundary had moved.
func modeDispatch(fn *ast.FuncDecl, deciders map[string]bool) (token.Pos, error) {
	vars := modeVariables(fn, deciders)

	var positions []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		s, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		if tag, ok := s.Tag.(*ast.Ident); ok && vars[tag.Name] {
			positions = append(positions, s.Pos())
		}
		return true
	})
	switch len(positions) {
	case 1:
		return positions[0], nil
	case 0:
		return 0, fmt.Errorf("%s switches on no mode-typed value, so the span has no closing "+
			"boundary; the dispatch was removed or the mode now travels under another name",
			fn.Name.Name)
	default:
		return 0, fmt.Errorf("%s holds %d switches on the mode, so which one closes the span is a "+
			"guess", fn.Name.Name, len(positions))
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

func position(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d", p.Filename, p.Line)
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
