package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func sortedMapKeys(m map[string]string) []string {
	i := 0
	keys := make([]string, len(m))
	for k := range m {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

func sortedMapValsByKeys(m map[string]string) []string {
	_, vals := sortedMapKeysAndVals(m)
	return vals
}

func sortedMapKeysAndVals(m map[string]string) ([]string, []string) {
	keys := sortedMapKeys(m)
	vals := make([]string, len(keys))
	for i, k := range keys {
		vals[i] = m[k]
	}
	return keys, vals
}

func longestStrInStringSlice(s []string) string {
	_longest := ""
	longest := &_longest
	for i := 0; i < len(s); i++ {
		if len(s[i]) > len(*longest) {
			longest = &s[i]
		}
	}
	return *longest
}

func mapInterfaceToStr(m map[string]interface{}) map[string]string {
	retVal := make(map[string]string)
	for key, value := range m {
		switch v := value.(type) {
		case int:
			retVal[key] = strconv.Itoa(v)
		case float64:
			retVal[key] = fmt.Sprintf("%f", v)
		case string:
			retVal[key] = v
		case map[string]interface{}:
			outJson, err := json.Marshal(v)
			if err != nil {
				return nil
			}
			retVal[key] = string(outJson)
		case []interface{}:
			outJson, err := json.Marshal(v)
			if err != nil {
				return nil
			}
			retVal[key] = string(outJson)
		default:
			retVal[key] = fmt.Sprintf("%v", v)
		}
	}
	return retVal
}

// Unmarshal []interface{} and map[string]interface{} stored as string
func unmarshStringJson(m map[string]interface{}) map[string]interface{} {
	retVal := make(map[string]interface{})
	for key, value := range m {
		switch v := value.(type) {
		case string:
			if strings.Contains(v, "[") && v[0] == '[' { //Check if the value is []interface{}
				var interfaceArr []interface{}
				_ = json.Unmarshal([]byte(v), &interfaceArr)
				if len(interfaceArr) > 0 {
					retVal[key] = interfaceArr
				}
			} else if strings.Contains(v, "{") && v[0] == '{' { //Check if the value is map[string]interface{}
				mapConv := make(map[string]interface{})
				_ = json.Unmarshal([]byte(v), &mapConv)
				if len(mapConv) > 0 {
					retVal[key] = mapConv
				}
			} else {
				retVal[key] = v
			}
		default:
			retVal[key] = v
		}
	}
	return retVal
}
