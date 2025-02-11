package util

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
)

func DefaultValue(defaultValue string, newValue string) string {
	if newValue == "" {
		return defaultValue
	}
	return newValue
}

func FormatBytes(bytes uint64) string {
	const (
		KB = 1 << 10 // 1024
		MB = 1 << 20 // 1024 * 1024
		GB = 1 << 30 // 1024 * 1024 * 1024
		TB = 1 << 40 // 1024 * 1024 * 1024 * 1024
	)

	var result string
	switch {
	case bytes >= TB:
		result = fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		result = fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		result = fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		result = fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		result = fmt.Sprintf("%d Byte", bytes)
	}

	return result
}

func MD5(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}

func GetCommand(command string) (string, error) {
	return exec.LookPath(command)
}

// ToJSON returns a json string
func ToJSON(v any) string {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		panic(err)
	}
	return buf.String()
}

// PrettyJSON returns a pretty formated json string
func PrettyJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return buf.String()
}
