package infra

// json_file.go — JSON 文件读写
// 对应 TS: src/infra/json-file.ts (23L)

import (
	"encoding/json"
	"os"
)

// ReadJSONFile 从文件读取 JSON 并反序列化到 dest。
// 对应 TS: readJsonFile(filePath)
func ReadJSONFile(filePath string, dest interface{}) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// WriteJSONFile 将对象序列化为 JSON 并写入文件（原子写入）。
// 对应 TS: writeJsonFile(filePath, value)
func WriteJSONFile(filePath string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteFileAtomic(filePath, data, 0o644)
}

// ReadJSONFileSafe 安全读取 JSON 文件（不存在返回 false）。
func ReadJSONFileSafe(filePath string, dest interface{}) bool {
	data, err := ReadFileSafe(filePath)
	if err != nil || data == nil {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}
