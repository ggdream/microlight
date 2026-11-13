package a2ui

import (
	"fmt"
	"strconv"
	"strings"
)

func (m *Model) template(source, scope, catalog string) (string, error) {
	var failure error
	result := interpolation.ReplaceAllStringFunc(source, func(part string) string {
		if strings.HasPrefix(part, "\\") {
			return part[1:]
		}
		value, err := m.expression(strings.TrimSpace(part[2:len(part)-1]), scope, catalog, 0)
		if err != nil && failure == nil {
			failure = err
		}
		return Text(value)
	})
	return result, failure
}

func (m *Model) expression(expr, scope, catalog string, depth int) (any, error) {
	if depth > 48 {
		return nil, fmt.Errorf("A2UI expression nesting too deep")
	}
	expr = strings.TrimSpace(expr)
	if name, args, ok := strings.Cut(expr, "("); ok && strings.HasSuffix(expr, ")") && !strings.HasPrefix(expr, "/") {
		call := FunctionCall{Name: strings.TrimSpace(name), Catalog: catalog, Args: make(map[string]any)}
		if call.Name == "@index" {
			call.Catalog = ""
		}
		for _, arg := range expressionArgs(args[:len(args)-1]) {
			key, raw, ok := strings.Cut(arg, ":")
			if !ok {
				return nil, fmt.Errorf("invalid template argument %q", arg)
			}
			value, err := m.expression(raw, scope, catalog, depth+1)
			if err != nil {
				return nil, err
			}
			call.Args[strings.TrimSpace(key)] = value
		}
		return m.evaluate(call, scope)
	}
	if len(expr) >= 2 && ((expr[0] == '\'' && expr[len(expr)-1] == '\'') || (expr[0] == '"' && expr[len(expr)-1] == '"')) {
		if expr[0] == '\'' {
			return strings.ReplaceAll(expr[1:len(expr)-1], `\'`, `'`), nil
		}
		return strconv.Unquote(expr)
	}
	switch expr {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if number, err := strconv.ParseFloat(expr, 64); err == nil {
		return number, nil
	}
	value, _ := m.Get(ResolvePath(scope, expr))
	return value, nil
}

func expressionArgs(args string) []string {
	var values []string
	start, depth := 0, 0
	var quote byte
	for i := 0; i < len(args); i++ {
		b := args[i]
		if quote != 0 {
			if b == '\\' {
				i++
			} else if b == quote {
				quote = 0
			}
			continue
		}
		switch b {
		case '\'', '"':
			quote = b
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				values = append(values, args[start:i])
				start = i + 1
			}
		}
	}
	if last := strings.TrimSpace(args[start:]); last != "" {
		values = append(values, last)
	}
	return values
}
