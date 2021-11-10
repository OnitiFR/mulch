package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const stringWordSeparators = "[ \t\n,.;:\\(\\)\\[\\]{}'\"/\\\\!\\?<>@#|*+-=]"

// TrueStr is the true truth.
const TrueStr = "true"
const FalseStr = "false"

// PathExist returns true if a file or directory exists
func PathExist(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	// I know Stat() may fail for a lot of reasons, but
	// os.IsNotExist is not enough, see ENOTDIR for
	// things like /etc/passwd/test
	if err != nil {
		return false
	}

	return true
}

// InterfaceValueToString converts most interface types to string
func InterfaceValueToString(iv interface{}) string {
	switch civ := iv.(type) {
	case int:
		return fmt.Sprintf("%d", civ)
	case int16:
		return fmt.Sprintf("%d", civ)
	case uint16:
		return fmt.Sprintf("%d", civ)
	case int32:
		return fmt.Sprintf("%d", civ)
	case int64:
		return strconv.FormatInt(civ, 10)
	case uint64:
		return strconv.FormatUint(civ, 10)
	case float32:
		return fmt.Sprintf("%f", civ)
	case float64:
		return strconv.FormatFloat(civ, 'f', -1, 64)
	case string:
		return civ
	case []byte:
		return string(civ)
	case bool:
		return strconv.FormatBool(civ)
	case time.Time:
		return civ.String()
	case time.Duration:
		return civ.String()
	case []string:
		return strings.Join(civ, ", ")
	}
	return "INVALID_TYPE"
}

// MapStringToInterface convert a map[string]string to a map[string]interface{}
func MapStringToInterface(ms map[string]string) map[string]interface{} {
	mi := make(map[string]interface{}, len(ms))
	for k, v := range ms {
		mi[k] = v
	}
	return mi
}

// StringFindVariables returns a deduplicated slice of all "variables" ($test)
// in the string
func StringFindVariables(str string) []string {
	re := regexp.MustCompile("\\$([a-zA-Z0-9_]+)(" + stringWordSeparators + "|$)")
	all := re.FindAllStringSubmatch(str, -1)

	// deduplicate using a map
	varMap := make(map[string]bool)
	for _, v := range all {
		varMap[v[1]] = true
	}

	// map to slice
	res := []string{}
	for name := range varMap {
		res = append(res, name)
	}
	return res
}

// StringExpandVariables expands "variables" ($test, for instance) in str
// and returns a new string
func StringExpandVariables(str string, variables map[string]interface{}) string {
	vars := StringFindVariables(str)
	for _, v := range vars {
		if val, exists := variables[v]; exists {
			re := regexp.MustCompile("\\$" + v + "(" + stringWordSeparators + "|$)")
			str = re.ReplaceAllString(str, InterfaceValueToString(val)+"${1}")
		}
	}
	return str
}

// FileContains returns true if file contain text
func FileContains(filepath string, text string) (bool, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return false, err
	}

	contains := strings.Contains(string(data), text)
	return contains, nil
}

// StringIsVariable returns true and the value if the string's like:
// FOOBAR=dummy
// (returns true and "dummy" if varName is "FOOBAR")
func StringIsVariable(s string, varName string) (bool, string) {
	if !strings.HasPrefix(s, varName+"=") {
		return false, ""
	}
	return true, s[len(varName)+1:]
}
