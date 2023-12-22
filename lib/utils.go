package lib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"syscall"
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
		default:
			outJson, err := json.Marshal(v)
			if err != nil {
				return nil
			}
			retVal[key] = string(outJson)
		}
	}
	return retVal
}

func findSectionSeparator(mmdbFile, sep string) (int64, error) {
	file, err := os.Open(mmdbFile)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}

	fileSize := fileInfo.Size()

	// Map the mmdb file into memory.
	mmap, err := syscall.Mmap(int(file.Fd()), 0, int(fileSize), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return 0, err
	}
	defer syscall.Munmap(mmap)

	// Search the last occurrence of the separator in the file.
	index := bytes.LastIndex(mmap, []byte(sep))
	if index != -1 {
		return int64(index), nil
	}

	return -1, nil
}
