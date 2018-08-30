package server

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
)

const stringWordSeparators = "[ \t\n,.;:\\(\\)\\[\\]{}'\"/\\\\!\\?<>@#|*+-=]"

// IsValidTokenName returns true is argument use only allowed chars for a token
// and does not start with a number
func IsValidTokenName(token string) bool {
	match, _ := regexp.MatchString("^[A-Za-z_][A-Za-z0-9_]*$", token)
	return match
}

// InterfaceValueToString converts most interface types to string
func InterfaceValueToString(iv interface{}) string {
	switch iv.(type) {
	case int:
		return fmt.Sprintf("%d", iv.(int))
	case int32:
		return fmt.Sprintf("%d", iv.(int32))
	case int64:
		return strconv.FormatInt(iv.(int64), 10)
	case float32:
		return fmt.Sprintf("%f", iv.(float32))
	case float64:
		return strconv.FormatFloat(iv.(float64), 'f', -1, 64)
	case string:
		return iv.(string)
	case []byte:
		return string(iv.([]byte))
	case bool:
		return strconv.FormatBool(iv.(bool))
	}
	return "INVALID_TYPE"
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
		if val, exists := variables[v]; exists == true {
			re := regexp.MustCompile("\\$" + v + "(" + stringWordSeparators + "|$)")
			str = re.ReplaceAllString(str, InterfaceValueToString(val)+"${1}")
		}
	}
	return str
}

// RandString generate a random string of A-Za-z0-9 runes
func RandString(n int, rand *rand.Rand) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
