package a2ui

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type ValidationResult struct {
	Valid    bool   `json:"valid"`
	Message  string `json:"message,omitempty"`
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity,omitempty"`
}

func Boolean(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case ValidationResult:
		return v.Valid
	case map[string]any:
		if valid, ok := v["valid"].(bool); ok {
			return valid
		}
	case json.Number, float64, int, int64:
		n := Number(value)
		return n != 0 && !math.IsNaN(n)
	}
	return false
}

func Number(value any) float64 {
	switch v := value.(type) {
	case bool:
		if v {
			return 1
		}
		return 0
	case json.Number, int, int64, float64:
		return Number(fmt.Sprint(v))
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0
		}
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return math.NaN()
}

func Text(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case map[string]any, []any, ValidationResult:
		data, _ := json.Marshal(value)
		return string(data)
	default:
		return fmt.Sprint(v)
	}
}

var interpolation = regexp.MustCompile(`\\?\$\{([^{}]*)\}`)
var dateTokens = strings.NewReplacer("yyyy", "2006", "MMMM", "January", "MMM", "Jan", "MM", "01", "dd", "02", "HH", "15", "mm", "04", "ss", "05", "EEEE", "Monday", "EEE", "Mon")

func (m *Model) basicFunction(call FunctionCall, scope string) (any, error) {
	args := call.Args
	value := args["value"]
	number, str := Number(value), Text(value)
	valid := func(ok bool, failure string) ValidationResult {
		if ok {
			return ValidationResult{Valid: true}
		}
		return ValidationResult{Message: failure}
	}
	bounded := func(n float64) bool {
		minimum, maximum := Number(args["min"]), Number(args["max"])
		return !math.IsNaN(n) && (math.IsNaN(minimum) || minimum <= n) && (math.IsNaN(maximum) || n <= maximum)
	}
	switch call.Name {
	case "required":
		ok := value != nil
		switch v := value.(type) {
		case string:
			ok = strings.TrimSpace(v) != ""
		case []any:
			ok = len(v) != 0
		}
		return valid(ok, "This field is required."), nil
	case "regex":
		matched, err := regexMatch(Text(args["pattern"]), str)
		if err != nil {
			return nil, err
		}
		return valid(matched, "The value does not match the expected pattern."), nil
	case "length":
		length := utf8.RuneCountInString(str)
		if list, ok := value.([]any); ok {
			length = len(list)
		}
		return valid(bounded(float64(length)), "The length is outside the permitted range."), nil
	case "numeric":
		return valid(bounded(number), "Enter a number in the permitted range."), nil
	case "email":
		email, err := mail.ParseAddress(str)
		_, domain, _ := strings.Cut(str, "@")
		result := valid(err == nil && email.Address == str && strings.LastIndex(domain, ".") >= 0 && len(domain)-strings.LastIndex(domain, ".") >= 3, "Please enter a valid email address (e.g. user@example.com).")
		if !result.Valid {
			result.Code = "INVALID_EMAIL"
		}
		return result, nil
	case "formatString":
		return m.template(str, scope, call.Catalog)
	case "formatNumber", "formatCurrency":
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return "", nil
		}
		decimals := 2
		if call.Name == "formatNumber" {
			decimals = -1
		}
		if n := Number(args["decimals"]); !math.IsNaN(n) && n >= 0 && n <= 20 {
			decimals = int(n)
		}
		formatted := strconv.FormatFloat(number, 'f', decimals, 64)
		if grouping, specified := args["grouping"]; !specified || Boolean(grouping) {
			tag := language.Make(m.Locale)
			if m.Locale == "" {
				tag = language.AmericanEnglish
			}
			if decimals < 0 {
				_, fraction, _ := strings.Cut(formatted, ".")
				decimals = len(fraction)
			}
			formatted = message.NewPrinter(tag).Sprintf("%.*f", decimals, number)
		}
		if call.Name == "formatCurrency" {
			currency := Text(args["currency"])
			symbol := map[string]string{"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "¥"}[currency]
			if symbol == "" {
				symbol = currency + " "
			}
			if strings.HasPrefix(formatted, "-") {
				return "-" + symbol + formatted[1:], nil
			}
			return symbol + formatted, nil
		}
		return formatted, nil
	case "formatDate":
		var parsed time.Time
		var err error
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", time.DateOnly, "15:04:05", "15:04"} {
			if parsed, err = time.Parse(layout, str); err == nil {
				break
			}
		}
		if err != nil {
			return "", nil
		}
		pattern := Text(args["format"])
		if pattern == "" {
			pattern = "yyyy-MM-dd"
		}
		if pattern == "ISO" {
			return parsed.UTC().Format("2006-01-02T15:04:05.000Z"), nil
		}
		return parsed.Format(dateTokens.Replace(pattern)), nil
	case "pluralize":
		form := "other"
		if number == 1 && !strings.HasPrefix(m.Locale, "zh") {
			form = "one"
		}
		if number == 0 && args["zero"] != nil {
			form = "zero"
		}
		if number == 2 && args["two"] != nil {
			form = "two"
		}
		result := args[form]
		if result == nil {
			result = args["other"]
		}
		return strings.NewReplacer("#", str, "{n}", str).Replace(Text(result)), nil
	case "and", "or":
		values, _ := args["values"].([]any)
		if len(values) < 2 {
			return nil, fmt.Errorf("%s requires at least two values", call.Name)
		}
		for _, v := range values {
			if Boolean(v) == (call.Name == "or") {
				return call.Name == "or", nil
			}
		}
		return call.Name == "and", nil
	case "not":
		return !Boolean(value), nil
	case "openUrl":
		return nil, fmt.Errorf("openUrl requires explicit user activation")
	case "@index":
		_, index, _ := strings.Cut(strings.TrimSuffix(scope, "/"), "/")
		if pos := strings.LastIndexByte(index, '/'); pos >= 0 {
			index = index[pos+1:]
		}
		n, err := strconv.Atoi(index)
		if err != nil || scope == "" {
			return nil, fmt.Errorf("@index requires a list item scope")
		}
		offset := Number(args["offset"])
		if math.IsNaN(offset) {
			offset = 0
		}
		return float64(n) + offset, nil
	default:
		return nil, fmt.Errorf("unknown A2UI function %q", call.Name)
	}
}

func SafeURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https":
		if u.Hostname() != "" && u.User == nil {
			return u, nil
		}
	case "mailto", "tel":
		if u.Opaque != "" {
			return u, nil
		}
	}
	return nil, fmt.Errorf("unsupported URL")
}
