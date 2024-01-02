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

type TypeSizes struct {
	PointerSize           int64
	Utf8StringSize        int64
	DoubleSize            int64
	BytesSize             int64
	Unsigned16bitIntSize  int64
	Unsigned32bitIntSize  int64
	Signed32bitIntSize    int64
	Unsigned64bitIntSize  int64
	Unsigned128bitIntSize int64
	MapKeyValueCount      int64
	ArrayLength           int64
	FloatSize             int64
}

func traverseDataSection(filePath string, startOffset, endOffset int64) (TypeSizes, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return TypeSizes{}, err
	}
	defer file.Close()

	// Go to the start offset of the data section.
	_, err = file.Seek(startOffset, 0)
	if err != nil {
		return TypeSizes{}, err
	}

	var typeSizes TypeSizes

	// Read and process bytes until the end offset is reached.
	for offset := startOffset; offset < endOffset; {
		var controlByte [1]byte
		_, err := file.Read(controlByte[:])
		if err != nil {
			return TypeSizes{}, err
		}
		offset++

		// Extract the type from the control byte.
		dataType := controlByte[0] & 0b111 // First three bits represent the type.
		// Check if it's an extended type.
		if dataType == 0 {
			// Read actual type number from the next byte
			var extendedTypeByte [1]byte
			_, err := file.Read(extendedTypeByte[:])
			if err != nil {
				return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
			}
			offset++
			payloadSize := int(controlByte[0] & 0b00011111) // Last five bits represent payload size
			switch extendedTypeByte[0] {
			case 1: // unsigned 32-bit int.

				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.Signed32bitIntSize += int64(payloadSize)

			case 2: // unsigned 64-bit int.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.Unsigned64bitIntSize += int64(payloadSize)

			case 3: // unsigned 128-bit int.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.Unsigned128bitIntSize += int64(payloadSize)

			case 4: // array.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.ArrayLength += int64(payloadSize)

			case 8: // float.
				typeSizes.FloatSize += 4
			}
		} else {
			payloadSize := int(controlByte[0] & 0b00011111) // Last five bits represent payload size.
			// Process based on the data type.
			switch dataType {
			case 1: // Pointer.
				size := int((controlByte[0] >> 3) & 0b00000011) // Extract the size bits at position 3 and 4.
				switch size {
				case 1:
					typeSizes.PointerSize += 1
				case 2:
					typeSizes.PointerSize += 2
				case 3:
					typeSizes.PointerSize += 3
				}

			case 2: // UTF-8 string.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.Utf8StringSize += int64(payloadSize)

			case 3: // Double.
				typeSizes.DoubleSize += 8

			case 4: // Byte.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.BytesSize += int64(payloadSize)

			case 5: // unsigned 16-bit int.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.Unsigned16bitIntSize += int64(payloadSize)

			case 6: // unsigned 32-bit int.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.Unsigned32bitIntSize += int64(payloadSize)

			case 7: // map.
				payloadSize, offset, err = payloadCalculation(file, payloadSize, offset)
				if err != nil {
					return TypeSizes{}, fmt.Errorf("couldn't read the file: %v", err)
				}
				typeSizes.MapKeyValueCount += int64(payloadSize)
			}
		}

	}

	return typeSizes, nil
}

// This is used for further calculation on the current payload size if it is either 29, 30 or 31.
func payloadCalculation(file *os.File, payloadSize int, offset int64) (int, int64, error) {
	if payloadSize == 29 {
		// Read the next byte as the payload size.
		var nextByte [1]byte
		_, err := file.Read(nextByte[:])
		if err != nil {
			return -1, -1, err
		}
		payloadSize = int(nextByte[0]) + 29
		offset++
	} else if payloadSize == 30 {
		// Read the next two bytes as the payload size.
		var nextBytes [2]byte
		_, err := file.Read(nextBytes[:])
		if err != nil {
			return -1, -1, err
		}
		payloadSize = int(nextBytes[0])<<8 + int(nextBytes[1]) + 285
		offset += 2
	} else if payloadSize == 31 {
		// Read the next three bytes as the payload size.
		var nextBytes [3]byte
		_, err := file.Read(nextBytes[:])
		if err != nil {
			return -1, -1, err
		}
		payloadSize = int(nextBytes[0])<<16 + int(nextBytes[1])<<8 + int(nextBytes[2]) + 65821
		offset += 3
	}

	return payloadSize, offset, nil
}
