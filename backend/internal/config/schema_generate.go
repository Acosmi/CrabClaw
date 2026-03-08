package config

// schema_generate.go — 从 Go struct 反射生成 JSON Schema (draft-07)
// 对应 TS 中 OpenAcosmiSchema.toJSONSchema({ target: "draft-07", unrepresentable: "any" })
//
// 使用 reflect 遍历 types.OpenAcosmiConfig 的字段结构，
// 读取 `json:` tag 生成与 TS Zod schema 等效的 JSON Schema。

import (
	"reflect"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// generateConfigSchema 生成 OpenAcosmiConfig 的 JSON Schema。
// 返回 map[string]interface{} 形式的 draft-07 JSON Schema。
func generateConfigSchema() map[string]interface{} {
	schema := reflectType(reflect.TypeOf(types.OpenAcosmiConfig{}))
	schema["title"] = "Crab Claw Config"
	schema["$schema"] = "http://json-schema.org/draft-07/schema#"
	return schema
}

// reflectType 递归生成给定 Go 类型的 JSON Schema。
func reflectType(t reflect.Type) map[string]interface{} {
	// 解引用指针
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		return reflectStruct(t)
	case reflect.Slice:
		return reflectSlice(t)
	case reflect.Map:
		return reflectMap(t)
	case reflect.String:
		return map[string]interface{}{"type": "string"}
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]interface{}{"type": "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]interface{}{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]interface{}{"type": "number"}
	case reflect.Interface:
		// interface{} / any — 对应 TS 的 unknown
		return map[string]interface{}{}
	default:
		return map[string]interface{}{}
	}
}

// reflectStruct 生成 struct 类型的 JSON Schema（type: object）。
func reflectStruct(t reflect.Type) map[string]interface{} {
	properties := make(map[string]interface{})
	var required []string

	collectStructFields(t, properties, &required)

	result := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		result["required"] = required
	}

	return result
}

// collectStructFields 递归收集结构体字段到 properties map 中。
// 对于嵌入（匿名）结构体字段，扁平化其属性到父级，匹配 Go JSON 序列化行为。
func collectStructFields(t reflect.Type, properties map[string]interface{}, required *[]string) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过非导出字段
		if !field.IsExported() {
			continue
		}

		// 处理嵌入（匿名）结构体 — 扁平化到父级（匹配 Go JSON 序列化行为）
		if field.Anonymous {
			ft := field.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				collectStructFields(ft, properties, required)
				continue
			}
		}

		// 解析 json tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name, opts := parseJSONTag(jsonTag)
		if name == "" {
			name = field.Name
		}

		// 生成字段 schema
		fieldSchema := reflectType(field.Type)

		// 如果字段是指针或有 omitempty，它是可选的
		// 否则加入 required
		isOptional := opts.contains("omitempty") || field.Type.Kind() == reflect.Ptr ||
			field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map ||
			field.Type.Kind() == reflect.Interface

		// 对指针类型标记 nullable
		if field.Type.Kind() == reflect.Ptr {
			fieldSchema["nullable"] = true
		}

		properties[name] = fieldSchema

		if !isOptional {
			*required = append(*required, name)
		}
	}
}

// reflectSlice 生成 slice 类型的 JSON Schema（type: array）。
func reflectSlice(t reflect.Type) map[string]interface{} {
	elemSchema := reflectType(t.Elem())
	return map[string]interface{}{
		"type":  "array",
		"items": elemSchema,
	}
}

// reflectMap 生成 map 类型的 JSON Schema（type: object + additionalProperties）。
func reflectMap(t reflect.Type) map[string]interface{} {
	valueSchema := reflectType(t.Elem())
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": valueSchema,
	}
}

// jsonTagOptions 解析后的 JSON tag 选项
type jsonTagOptions string

func (o jsonTagOptions) contains(opt string) bool {
	return strings.Contains(string(o), opt)
}

// parseJSONTag 解析 `json:"name,omitempty"` 格式的标签。
func parseJSONTag(tag string) (string, jsonTagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], jsonTagOptions(tag[idx+1:])
	}
	return tag, ""
}
