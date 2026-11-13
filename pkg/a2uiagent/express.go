package a2uiagent

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/scanner"
	"unicode"

	"github.com/ggdream/microlight/pkg/a2ui"
)

type reference string
type call struct {
	name string
	args []argument
}
type argument struct {
	name  string
	value any
}
type statement struct {
	target string
	value  any
}

type expressParser struct {
	scanner.Scanner
	token rune
	err   error
	depth int
}

func parseExpress(source string) ([]statement, error) {
	p := &expressParser{}
	p.Init(strings.NewReader(expressSource(source)))
	p.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanRawStrings | scanner.ScanInts | scanner.ScanFloats | scanner.ScanComments | scanner.SkipComments
	p.Error = func(_ *scanner.Scanner, message string) { p.fail(message) }
	p.next()
	var statements []statement
	for p.token != scanner.EOF && p.err == nil {
		if p.take(';') {
			continue
		}
		value := p.expression()
		if p.take('=') {
			switch target := value.(type) {
			case reference:
				statements = append(statements, statement{string(target), p.expression()})
			case a2ui.Binding:
				statements = append(statements, statement{"$" + target.Path, p.expression()})
			default:
				p.fail("invalid assignment target")
			}
		} else {
			statements = append(statements, statement{value: value})
		}
	}
	return statements, p.err
}

func (p *expressParser) next() { p.token = p.Scan() }
func (p *expressParser) take(token rune) bool {
	if p.token != token {
		return false
	}
	p.next()
	return true
}
func (p *expressParser) fail(message string) {
	if p.err == nil {
		p.err = fmt.Errorf("Express %s: %s", p.Position, message)
	}
}
func (p *expressParser) require(token rune) {
	if !p.take(token) {
		p.fail("expected " + string(token))
	}
}

func (p *expressParser) expression() any {
	if p.depth >= 96 {
		p.fail("expression nesting too deep")
		return nil
	}
	p.depth++
	defer func() { p.depth-- }()
	switch p.token {
	case '[':
		p.next()
		var values []any
		for p.token != ']' && p.err == nil {
			values = append(values, p.expression())
			if !p.take(',') {
				break
			}
		}
		p.require(']')
		if values == nil {
			values = []any{}
		}
		return values
	case '{':
		p.next()
		values := make(map[string]any)
		for p.token != '}' && p.err == nil {
			key := p.TokenText()
			if p.token == scanner.String {
				var err error
				key, err = strconv.Unquote(key)
				if err != nil {
					p.fail(err.Error())
				}
			} else if p.token != scanner.Ident {
				p.fail("expected object key")
			}
			p.next()
			p.require(':')
			values[key] = p.expression()
			if !p.take(',') {
				break
			}
		}
		p.require('}')
		return values
	case '$':
		// Read the adjacent path directly; whitespace and delimiters end it.
		var path strings.Builder
		for r := p.Peek(); r == '/' || r == '_' || r == '~' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'; r = p.Peek() {
			path.WriteRune(p.Next())
		}
		p.next()
		return a2ui.Binding{Path: path.String()}
	case scanner.String, scanner.RawString:
		value, err := strconv.Unquote(p.TokenText())
		p.next()
		if err != nil {
			p.fail(err.Error())
		}
		return value
	case scanner.Int, scanner.Float, '+', '-':
		prefix := ""
		if p.token == '+' || p.token == '-' {
			prefix = p.TokenText()
			p.next()
		}
		value, err := strconv.ParseFloat(prefix+p.TokenText(), 64)
		p.next()
		if err != nil {
			p.fail(err.Error())
		}
		return value
	case scanner.Ident, '@', '?':
		prefix := ""
		if p.token == '@' || p.token == '?' {
			prefix = p.TokenText()
			p.next()
		}
		name := prefix + p.TokenText()
		if p.token != scanner.Ident {
			p.fail("expected identifier")
			return nil
		}
		p.next()
		if !p.take('(') {
			switch name {
			case "true":
				return true
			case "false":
				return false
			case "null":
				return nil
			}
			if strings.HasPrefix(name, "?") {
				return call{name: name}
			}
			return reference(name)
		}
		value := call{name: name}
		for p.token != ')' && p.err == nil {
			arg := argument{value: p.expression()}
			if p.take('=') {
				ref, ok := arg.value.(reference)
				if !ok {
					p.fail("invalid named argument")
					break
				}
				arg.name, arg.value = string(ref), p.expression()
			}
			value.args = append(value.args, arg)
			if !p.take(',') {
				break
			}
		}
		p.require(')')
		return value
	default:
		p.fail("unexpected token " + p.TokenText())
		return nil
	}
}

type expressCompiler struct {
	catalogs     []*a2ui.Catalog
	pinned       *a2ui.Catalog
	surface      string
	variables    map[string]any
	components   []a2ui.Component
	building     map[string]bool
	allowForward bool
	ids          map[string]bool
}

// CompileExpress lowers Express component declarations and data assignments to
// wire messages. The state determines whether a surface already exists.
func CompileExpress(source string, catalog *a2ui.Catalog, state *a2ui.Processor) ([]a2ui.AgentMessage, error) {
	return CompileExpressCatalogs(source, []*a2ui.Catalog{catalog}, state)
}

// CompileExpressCatalogs resolves Express declarations using the negotiated
// catalogs in order. A surface can explicitly pin one of those catalogs.
func CompileExpressCatalogs(source string, catalogs []*a2ui.Catalog, state *a2ui.Processor) ([]a2ui.AgentMessage, error) {
	return compileExpress(source, catalogs, state, false)
}

func compileExpress(source string, catalogs []*a2ui.Catalog, state *a2ui.Processor, allowForward bool) ([]a2ui.AgentMessage, error) {
	if len(source) > maxBlock || len(catalogs) == 0 {
		return nil, fmt.Errorf("invalid Express source or catalog")
	}
	for _, catalog := range catalogs {
		if catalog == nil {
			return nil, fmt.Errorf("missing Express catalog")
		}
	}
	statements, err := parseExpress(source)
	if err != nil {
		return nil, err
	}
	if len(statements) == 0 {
		return nil, fmt.Errorf("empty Express block")
	}
	var messages []a2ui.AgentMessage
	start := 0
	for i, statement := range statements {
		if fn, ok := statement.value.(call); i > start && statement.target == "" && ok && fn.name == "surface" {
			part, err := compileExpressScope(statements[start:i], catalogs, state, allowForward)
			if err != nil {
				return nil, err
			}
			messages = append(messages, part...)
			for _, message := range part {
				if message.Delete != nil && state != nil {
					state = state.Fork()
					if err := state.Apply(message); err != nil {
						return nil, err
					}
				}
			}
			start = i
		}
	}
	part, err := compileExpressScope(statements[start:], catalogs, state, allowForward)
	return append(messages, part...), err
}

func compileExpressScope(statements []statement, catalogs []*a2ui.Catalog, state *a2ui.Processor, allowForward bool) ([]a2ui.AgentMessage, error) {
	c := expressCompiler{catalogs: catalogs, surface: "default_surface", variables: make(map[string]any), building: make(map[string]bool), ids: make(map[string]bool), allowForward: allowForward}
	if len(catalogs) == 1 {
		c.pinned = catalogs[0]
	}
	var metadata *a2ui.Metadata
	updateScope := false
	sendDataModel := false
	callIndex := 0
	explicitSurface := false
	for _, s := range statements {
		if s.target != "" && !strings.HasPrefix(s.target, "$") {
			c.variables[s.target] = s.value
		}
	}
	var updates []a2ui.AgentMessage
	var model a2ui.Model
	_ = model.Set("", map[string]any{})
	for _, s := range statements {
		if strings.HasPrefix(s.target, "$") {
			v, err := c.value(s.value, "", nil)
			if err != nil {
				return nil, err
			}
			path := strings.TrimPrefix(s.target, "$")
			if path == "" || path == "/" {
				if _, ok := v.(map[string]any); !ok {
					return nil, fmt.Errorf("surface data root requires an object")
				}
			}
			if err := model.Set(path, v); err != nil {
				return nil, err
			}
			encoded, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			updates = append(updates, a2ui.AgentMessage{Version: a2ui.Version, Data: &a2ui.UpdateDataModel{Path: path, Value: encoded}})
			continue
		}
		fn, ok := s.value.(call)
		if !ok {
			continue
		}
		if s.target != "" {
			if c.componentCatalog(fn) != nil {
				if err := c.component(s.target, fn); err != nil {
					return nil, err
				}
			} else if first := []rune(fn.name); len(first) > 0 && unicode.IsUpper(first[0]) && fn.name != "Event" {
				return nil, fmt.Errorf("unknown component %q", fn.name)
			} else if _, explicit := expressCatalogArg(fn); explicit || strings.HasPrefix(fn.name, "@") {
				if _, err := c.value(fn, "", nil); err != nil {
					return nil, err
				}
			}
			continue
		}
		switch fn.name {
		case "surface":
			if len(fn.args) == 0 {
				return nil, fmt.Errorf("surface requires an ID")
			}
			id, ok := fn.args[0].value.(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("invalid Express surface ID")
			}
			c.surface = id
			if state != nil {
				if surface := state.Surface(id); surface != nil && surface.Catalog() != "" {
					c.pinned = c.findCatalog(surface.Catalog())
					if c.pinned == nil {
						return nil, fmt.Errorf("unknown surface catalog %s", surface.Catalog())
					}
				}
			}
			explicitSurface = true
			for i, arg := range fn.args {
				if i == 0 {
					continue
				}
				v, err := c.value(arg.value, "", nil)
				if err != nil {
					return nil, err
				}
				switch {
				case arg.name == "catalogId" || arg.name == "catalog" || (i == 1 && arg.name == ""):
					if i == 1 && arg.name == "" && len(catalogs) > 1 {
						return nil, fmt.Errorf("specify a named catalogId for a surface")
					}
					catalogID, _ := v.(string)
					c.pinned = c.findCatalog(catalogID)
					if c.pinned == nil {
						return nil, fmt.Errorf("unknown surface catalog %v", v)
					}
				case arg.name == "sendDataModel":
					sendDataModel = a2ui.Boolean(v)
				case arg.name == "update":
					updateScope = a2ui.Boolean(v)
				case arg.name == "extensions":
					ext, ok := v.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid surface extensions")
					}
					metadata = &a2ui.Metadata{Extensions: ext}
				}
			}
		case "deleteSurface":
			id := c.surface
			if len(fn.args) > 0 {
				id, _ = fn.args[0].value.(string)
			}
			updates = append(updates, a2ui.AgentMessage{Version: a2ui.Version, Delete: &a2ui.SurfaceRef{ID: id}})
		default:
			v, err := c.value(fn, "", nil)
			if err != nil {
				return nil, err
			}
			raw, _ := json.Marshal(v)
			var function a2ui.FunctionCall
			if err := json.Unmarshal(raw, &function); err != nil {
				return nil, err
			}
			if strings.HasPrefix(function.Name, "@") {
				return nil, fmt.Errorf("system functions require a component binding")
			}
			catalog := c.functionCatalog(function.Name, function.Catalog)
			if catalog == nil || !catalog.AgentMayCall(function.Name) {
				return nil, fmt.Errorf("agent may not call %q", function.Name)
			}
			function.Catalog = catalog.ID
			callIndex++
			surface := ""
			if explicitSurface {
				surface = c.surface
			}
			updates = append(updates, a2ui.AgentMessage{Version: a2ui.Version, CallRenderer: &a2ui.CallRendererFunction{ID: fmt.Sprintf("call_%d", callIndex), Surface: surface, Function: function}})
		}
	}
	id := a2ui.SurfaceRef{ID: c.surface}
	creating := !updateScope && c.ids[a2ui.Root]
	if creating && state != nil && state.Surface(c.surface) != nil {
		return nil, fmt.Errorf("surface %q already exists", c.surface)
	}
	if metadata != nil && !creating {
		return nil, fmt.Errorf("only new surfaces can have extensions")
	}
	var messages []a2ui.AgentMessage
	if creating && len(c.components) > 0 {
		raw, _ := model.Get("")
		data, _ := raw.(map[string]any)
		messages = append(messages, a2ui.AgentMessage{Version: a2ui.Version, Create: &a2ui.CreateSurface{SurfaceRef: id, Catalog: catalogID(c.pinned), Components: c.components, DataModel: data, Metadata: metadata, SendDataModel: sendDataModel}})
	} else if len(c.components) > 0 {
		messages = append(messages, a2ui.AgentMessage{Version: a2ui.Version, Components: &a2ui.UpdateComponents{SurfaceRef: id, Components: c.components}})
	}
	for _, update := range updates {
		if update.Data != nil {
			if creating && len(c.components) > 0 {
				continue
			}
			update.Data.SurfaceRef = id
		}
		messages = append(messages, update)
	}
	return messages, nil
}

func catalogID(catalog *a2ui.Catalog) string {
	if catalog == nil {
		return ""
	}
	return catalog.ID
}

func (c *expressCompiler) findCatalog(id string) *a2ui.Catalog {
	for _, catalog := range c.catalogs {
		if catalog.ID == id {
			return catalog
		}
	}
	return nil
}

func (c *expressCompiler) componentCatalog(fn call) *a2ui.Catalog {
	name := fn.name
	id, _ := expressCatalogArg(fn)
	if id != "" {
		catalog := c.findCatalog(id)
		if catalog != nil && catalog.Components[name] != nil {
			return catalog
		}
		return nil
	}
	for _, catalog := range c.catalogs {
		if (c.pinned == nil || c.pinned == catalog) && catalog.Components[name] != nil {
			return catalog
		}
	}
	return nil
}

func (c *expressCompiler) functionCatalog(name, id string) *a2ui.Catalog {
	for _, catalog := range c.catalogs {
		if ((id == "" && (c.pinned == nil || c.pinned == catalog)) || catalog.ID == id) && catalog.Functions[name] != nil {
			return catalog
		}
	}
	return nil
}

func (c *expressCompiler) component(id string, fn call) error {
	id, err := explicitComponentID(id, fn)
	if err != nil {
		return err
	}
	if c.ids[id] {
		return fmt.Errorf("duplicate component ID %q", id)
	}
	c.ids[id] = true
	catalog := c.componentCatalog(fn)
	if catalog == nil {
		return fmt.Errorf("unknown component %q", fn.name)
	}
	raw := catalog.Components[fn.name]
	fields := slices.DeleteFunc(schemaFields(raw, "component"), func(name string) bool { return name == "catalogId" })
	if strings.Contains(string(raw), "Checkable") {
		fields = append(fields, "checks")
	}
	var checks []any
	args := make([]argument, 0, len(fn.args))
	for _, arg := range fn.args {
		if arg.name == "" {
			switch value := arg.value.(type) {
			case call:
				if strings.HasPrefix(value.name, "?") {
					checks = append(checks, value)
					continue
				}
			case []any:
				if len(value) > 0 && allExpressChecks(value) {
					checks = append(checks, value...)
					continue
				}
			}
		}
		args = append(args, arg)
	}
	if len(checks) != 0 {
		args = append(args, argument{name: "checks", value: checks})
	}
	fn.args = args
	fn = placeNumericArgument(fn, fields, raw)
	pos, err := orderedArgs(fn, fields)
	if err != nil {
		return err
	}
	index := len(c.components)
	c.components = append(c.components, a2ui.Component{})
	props := make(map[string]any)
	for _, arg := range pos {
		if ref, ok := arg.value.(reference); ok && ref == "_" {
			continue
		}
		if containsBoundLiteral(arg.value) {
			var property json.RawMessage
			propertySchema(raw, arg.name, &property)
			if strings.Contains(string(property), "Literal") || containsCallLikeLiteral(arg.value) {
				return fmt.Errorf("literal object cannot contain A2UI bindings")
			}
		}
		if arg.name == "extensions" && containsInvocation(arg.value) {
			return fmt.Errorf("extensions must contain literal values")
		}
		if arg.name == "id" || arg.name == "catalogId" {
			continue
		}
		var childSchema json.RawMessage
		propertySchema(raw, arg.name, &childSchema)
		value := arg.value
		if c.allowForward {
			value = c.forward(value, childSchema)
		}
		v, err := c.value(value, id+"_"+arg.name, pos)
		if err != nil {
			return err
		}
		name := arg.name
		if name == "componentCatalogId" {
			name = "catalogId"
		}
		if name == "checks" {
			name = "@checks"
		}
		var definition json.RawMessage
		propertySchema(raw, name, &definition)
		if name == "action" || strings.Contains(string(definition), "/Action") {
			if fn, ok := v.(a2ui.FunctionCall); ok {
				v = map[string]any{"functionCall": fn}
			}
		}
		if name == "extensions" {
			name = "@metadata"
			v = map[string]any{"extensions": v}
		}
		props[name] = v
	}
	component, err := a2ui.NewComponent(id, fn.name, props)
	if err != nil {
		return err
	}
	if c.pinned == nil || c.pinned != catalog {
		component.Catalog = catalog.ID
	}
	if err := catalog.ValidateComponent(component); err != nil {
		return err
	}
	c.components[index] = component
	return nil
}

func (c *expressCompiler) value(value any, id string, componentArgs []argument) (any, error) {
	switch v := value.(type) {
	case reference:
		name := string(v)
		raw, ok := c.variables[name]
		if !ok {
			return nil, fmt.Errorf("unknown Express variable %q", name)
		}
		if fn, ok := raw.(call); ok && c.componentCatalog(fn) != nil {
			return explicitComponentID(name, fn)
		}
		if c.building[name] {
			return nil, fmt.Errorf("cyclic Express variable %q", name)
		}
		c.building[name] = true
		defer delete(c.building, name)
		return c.value(raw, id, componentArgs)
	case call:
		if c.componentCatalog(v) != nil {
			if id == "" {
				return nil, fmt.Errorf("inline component requires a parent")
			}
			return id, c.component(id, v)
		}
		if v.name == "Event" || v.name == "_template" {
			if _, specified := expressCatalogArg(v); specified {
				return nil, fmt.Errorf("%s cannot specify catalogId", v.name)
			}
			fields := []string{"name", "context", "userMessage"}
			if v.name == "_template" {
				fields = []string{"path", "componentId"}
			}
			args, err := orderedArgs(v, fields)
			if err != nil {
				return nil, err
			}
			object := make(map[string]any)
			for _, arg := range args {
				item, err := c.value(arg.value, id+"_"+arg.name, componentArgs)
				if err != nil {
					return nil, err
				}
				if binding, ok := item.(a2ui.Binding); ok && v.name == "_template" {
					item = binding.Path
				}
				object[arg.name] = item
			}
			if v.name == "Event" {
				return map[string]any{"event": object}, nil
			}
			return object, nil
		}
		check := strings.HasPrefix(v.name, "?")
		name := strings.TrimPrefix(v.name, "?")
		var fields []string
		if name == "@index" {
			for _, arg := range v.args {
				if arg.name != "" && arg.name != "offset" {
					return nil, fmt.Errorf("invalid @index argument %s", arg.name)
				}
			}
			fields = []string{"offset"}
		}
		checkMessage := ""
		checkCatalog, explicitCatalog := expressCatalogArg(v)
		if explicitCatalog {
			if checkCatalog == "" {
				return nil, fmt.Errorf("invalid catalogId for %s", name)
			}
			v.args = slices.DeleteFunc(slices.Clone(v.args), func(arg argument) bool { return arg.name == "catalogId" })
		}
		if check {
			if len(v.args) > 0 {
				if options, ok := v.args[len(v.args)-1].value.(map[string]any); ok {
					if value, exists := options["catalogId"]; exists {
						checkCatalog, ok = value.(string)
						if !ok || checkCatalog == "" || len(options) != 1 {
							return nil, fmt.Errorf("invalid check catalog")
						}
						v.args = v.args[:len(v.args)-1]
					}
				}
			}
		}
		var catalog *a2ui.Catalog
		if name != "@index" {
			catalog = c.functionCatalog(name, checkCatalog)
			if catalog == nil {
				return nil, fmt.Errorf("unknown Express function %q in catalog %q", name, checkCatalog)
			}
			fields = schemaFields(functionArgs(catalog.Functions[name]), "")
			if c.pinned == nil {
				checkCatalog = catalog.ID
			}
		} else if checkCatalog != "" {
			return nil, fmt.Errorf("system function %s cannot specify catalogId", name)
		}
		if check {
			for _, arg := range componentArgs {
				if arg.name == "value" {
					v.args = append([]argument{{value: arg.value}}, v.args...)
					break
				}
			}
			if len(v.args) > len(fields) && len(v.args) > 0 {
				checkMessage, _ = v.args[len(v.args)-1].value.(string)
				v.args = v.args[:len(v.args)-1]
			}
		}
		args, err := orderedArgs(v, fields)
		if err != nil {
			return nil, err
		}
		call := a2ui.FunctionCall{Name: name, Catalog: checkCatalog}
		if len(args) > 0 {
			call.Args = make(map[string]any)
		}
		for _, arg := range args {
			item, err := c.value(arg.value, id+"_"+arg.name, componentArgs)
			if err != nil {
				return nil, err
			}
			call.Args[arg.name] = item
		}
		if name != "@index" {
			if err := catalog.ValidateFunction(call); err != nil {
				return nil, err
			}
		}
		if check {
			result := map[string]any{"condition": call}
			if checkMessage == "" {
				checkMessage = defaultCheckMessage(name)
			}
			result["message"] = checkMessage
			return result, nil
		}
		return call, nil
	case []any:
		items := make([]any, len(v))
		for i, item := range v {
			next, err := c.value(item, fmt.Sprintf("%s_%d", id, i), componentArgs)
			if err != nil {
				return nil, err
			}
			items[i] = next
		}
		return items, nil
	case map[string]any:
		items := make(map[string]any, len(v))
		for key, item := range v {
			next, err := c.value(item, id+"_"+key, componentArgs)
			if err != nil {
				return nil, err
			}
			if key == "action" {
				if fn, ok := next.(a2ui.FunctionCall); ok {
					next = map[string]any{"functionCall": fn}
				}
			}
			items[key] = next
		}
		return items, nil
	default:
		return value, nil
	}
}

func allExpressChecks(values []any) bool {
	for _, value := range values {
		call, ok := value.(call)
		if !ok || !strings.HasPrefix(call.name, "?") {
			return false
		}
	}
	return true
}

func orderedArgs(fn call, fields []string) ([]argument, error) {
	args := append([]argument(nil), fn.args...)
	seen := make(map[string]bool)
	for i := range args {
		if args[i].name == "" {
			if i >= len(fields) {
				return nil, fmt.Errorf("too many arguments for %s", fn.name)
			}
			args[i].name = fields[i]
		}
		if seen[args[i].name] {
			return nil, fmt.Errorf("duplicate argument %q", args[i].name)
		}
		seen[args[i].name] = true
	}
	return args, nil
}

func functionArgs(raw json.RawMessage) json.RawMessage {
	var node struct{ Properties map[string]json.RawMessage }
	_ = json.Unmarshal(raw, &node)
	return node.Properties["args"]
}

func schemaFields(raw json.RawMessage, exclude string) []string {
	var node struct {
		Properties json.RawMessage
		AllOf      []json.RawMessage
	}
	_ = json.Unmarshal(raw, &node)
	var result []string
	for _, part := range node.AllOf {
		result = append(result, schemaFields(part, exclude)...)
	}
	d := json.NewDecoder(strings.NewReader(string(node.Properties)))
	if _, err := d.Token(); err != nil {
		return result
	}
	for d.More() {
		key, err := d.Token()
		if err != nil && err != io.EOF {
			break
		}
		name, ok := key.(string)
		if !ok {
			break
		}
		if name != exclude {
			result = append(result, name)
		}
		if d.Decode(new(json.RawMessage)) != nil {
			break
		}
	}
	return result
}

func validIdentifier(value string) bool {
	for i, r := range value {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || (!unicode.IsDigit(r) && !unicode.IsMark(r))) {
			return false
		}
	}
	return value != ""
}

func containsInvocation(value any) bool {
	switch v := value.(type) {
	case call:
		return true
	case []any:
		for _, item := range v {
			if containsInvocation(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if containsInvocation(item) {
				return true
			}
		}
	}
	return false
}

func placeNumericArgument(fn call, fields []string, raw json.RawMessage) call {
	args := append([]argument(nil), fn.args...)
	for i, arg := range args {
		if arg.name != "" || i >= len(fields) || i != len(args)-1 {
			continue
		}
		if _, ok := arg.value.(float64); !ok {
			continue
		}
		var current json.RawMessage
		required := propertySchema(raw, fields[i], &current)
		if required || !strings.Contains(string(current), `"enum"`) {
			continue
		}
		var match string
		for _, field := range fields[i+1:] {
			var definition json.RawMessage
			propertySchema(raw, field, &definition)
			var props struct {
				Type string
				Ref  string `json:"$ref"`
			}
			_ = json.Unmarshal(definition, &props)
			if props.Type == "number" || props.Type == "integer" || strings.HasSuffix(props.Ref, "/DynamicNumber") {
				if match != "" {
					match = ""
					break
				}
				match = field
			}
		}
		if match != "" {
			args[i].name = match
		}
	}
	fn.args = args
	return fn
}

func propertySchema(raw json.RawMessage, name string, result *json.RawMessage) bool {
	var node struct {
		Properties map[string]json.RawMessage
		Required   []string
		AllOf      []json.RawMessage
	}
	_ = json.Unmarshal(raw, &node)
	if item, ok := node.Properties[name]; ok {
		*result = item
		return slices.Contains(node.Required, name)
	}
	for _, part := range node.AllOf {
		required := propertySchema(part, name, result)
		if *result != nil {
			return required
		}
	}
	return false
}

func containsBoundLiteral(value any) bool {
	switch v := value.(type) {
	case a2ui.Binding:
		return true
	case []any:
		for _, item := range v {
			if containsBoundLiteral(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if containsBoundLiteral(item) {
				return true
			}
		}
	}
	return false
}

func containsCallLikeLiteral(value any) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if containsCallLikeLiteral(item) {
				return true
			}
		}
	case map[string]any:
		if _, ok := v["@call"]; ok {
			return true
		}
		for _, item := range v {
			if containsCallLikeLiteral(item) {
				return true
			}
		}
	}
	return false
}

func explicitComponentID(fallback string, fn call) (string, error) {
	for _, arg := range fn.args {
		if arg.name != "id" {
			continue
		}
		id, ok := arg.value.(string)
		if !ok || !validIdentifier(id) {
			return "", fmt.Errorf("invalid component id")
		}
		return id, nil
	}
	return fallback, nil
}

func (c *expressCompiler) forward(value any, schema json.RawMessage) any {
	switch v := value.(type) {
	case reference:
		if strings.Contains(string(schema), "/Child") && v != "_" {
			if _, exists := c.variables[string(v)]; !exists {
				return string(v)
			}
		}
	case []any:
		next := append([]any(nil), v...)
		var node struct{ Items json.RawMessage }
		_ = json.Unmarshal(schema, &node)
		itemSchema := node.Items
		if itemSchema == nil {
			itemSchema = schema
		}
		for i, item := range next {
			next[i] = c.forward(item, itemSchema)
		}
		return next
	case map[string]any:
		next := make(map[string]any, len(v))
		for key, item := range v {
			var child json.RawMessage
			propertySchema(schema, key, &child)
			next[key] = c.forward(item, child)
		}
		return next
	}
	return value
}

func expressCatalogArg(fn call) (string, bool) {
	for _, arg := range fn.args {
		if arg.name == "catalogId" {
			id, _ := arg.value.(string)
			return id, true
		}
	}
	return "", false
}
