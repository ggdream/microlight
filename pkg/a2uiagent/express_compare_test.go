package a2uiagent

import (
	"reflect"
	"strings"
	"testing"
)

func TestExpressObjectOrderHasNoSemanticEffect(t *testing.T) {
	one, err := parseExpress(`root = Text(formatString({a: 1, b: 2}))`)
	if err != nil {
		t.Fatal(err)
	}
	two, err := parseExpress(`root = Text(formatString({b: 2, a: 1}))`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, two) {
		t.Fatal("map key order changed the Express AST")
	}
}

// ContainsExpressFragment compares complete statements after ignoring object
// property order. It is used for formatting assertions in conformance tests.
func ContainsExpressFragment(source, fragment string) bool {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return true
	}
	expect, err := parseExpress(fragment)
	if err != nil || len(expect) != 1 {
		return false
	}
	statements, err := parseExpress(source)
	if err != nil {
		return false
	}
	for _, statement := range statements {
		if reflect.DeepEqual(statement, expect[0]) || expressHasSubexpression(statement.value, expect[0].value) && expect[0].target == "" {
			return true
		}
	}
	return false
}

func expressHasSubexpression(value, target any) bool {
	if reflect.DeepEqual(value, target) {
		return true
	}
	switch v := value.(type) {
	case call:
		for _, arg := range v.args {
			if expressHasSubexpression(arg.value, target) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if expressHasSubexpression(item, target) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if expressHasSubexpression(item, target) {
				return true
			}
		}
	}
	return false
}
