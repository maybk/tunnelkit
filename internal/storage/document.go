package storage

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// StripUTF8BOM 去除跨平台复制或编辑可能引入的 UTF-8 BOM 前缀。
func StripUTF8BOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}

// MarshalDocument 写入带格式版本的 JSON；版本由所属模块独立维护。
func MarshalDocument(version int, value any) ([]byte, error) {
	if version < 1 {
		return nil, errors.New("无效的数据格式版本")
	}
	payload, err := json.Marshal(value, json.FormatNilSliceAsNull(true), json.FormatNilMapAsNull(true))
	if err != nil {
		return nil, err
	}
	if bytes.Equal(payload, []byte("null")) {
		return nil, errors.New("数据载荷不能为空")
	}
	return json.Marshal(struct {
		SchemaVersion int            `json:"schemaVersion"`
		Data          jsontext.Value `json:"data"`
	}{version, payload}, jsontext.WithIndent("  "))
}

// DecodeDocument 分离版本与载荷。无版本数据交给所属模块识别，未知版本不尝试降级。
func DecodeDocument(raw []byte, current int) (payload []byte, legacy bool, err error) {
	raw = StripUTF8BOM(raw)
	var fields map[string]jsontext.Value
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		if err := json.Unmarshal(trimmed, &fields); err != nil {
			return nil, false, err
		}
		if versionRaw, ok := fields["schemaVersion"]; ok {
			var version int
			if err := json.Unmarshal(versionRaw, &version); err != nil {
				return nil, false, err
			}
			if version != current || version < 1 {
				return nil, false, fmt.Errorf("不支持的数据格式版本 %d（当前支持 %d）", version, current)
			}
			data, ok := fields["data"]
			if !ok || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
				return nil, false, errors.New("数据文件缺少有效载荷")
			}
			if len(fields) != 2 {
				return nil, false, errors.New("数据文件包含未知封套字段")
			}
			return data, false, nil
		}
	}
	return raw, true, nil
}
