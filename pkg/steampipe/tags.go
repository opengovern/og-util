package steampipe

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"reflect"

	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"

	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
)

func ExtractTagsAndNames(plg *plugin.Plugin, logger *zap.Logger, pluginTableName, resourceType string, source interface{}, descriptionMap map[string]interface{}) (map[string]string, string, error) {
	var cells map[string]*proto.Column

	desc, err := ConvertToDescription(logger, resourceType, source, descriptionMap)
	if err != nil {
		if logger != nil {
			logger.Error("Error converting to description", zap.Error(err), zap.String("resourceType", resourceType), zap.Any("source", source))
		}
		return nil, "", err
	}

	cells, err = DescriptionToRecord(logger, plg, desc, pluginTableName)
	if err != nil {
		if logger != nil {
			logger.Error("Error converting to record", zap.Error(err), zap.String("resourceType", resourceType), zap.Any("source", source))
		}
		return nil, "", err
	}

	tags := map[string]string{}
	var name string
	for k, v := range cells {
		if k == "title" || k == "name" {
			name = v.GetStringValue()
		}
		if k == "tags" {
			if jsonBytes := v.GetJsonValue(); jsonBytes != nil && len(jsonBytes) > 0 &&
				string(jsonBytes) != "null" && string(jsonBytes) != "[]" &&
				string(jsonBytes) != "{}" && string(jsonBytes) != "\"\"" {
				var t interface{}
				err := json.Unmarshal(jsonBytes, &t)
				if err != nil {
					if logger != nil {
						logger.Error("Error unmarshalling tags", zap.Error(err), zap.String("resourceType", resourceType), zap.Any("source", source))
					}
					return nil, "", err
				}

				if tmap, ok := t.(map[string]string); ok {
					tags = tmap
				} else if t == nil {
					return tags, "", nil
				} else if tmap, ok := t.(map[string]interface{}); ok {
					for tk, tv := range tmap {
						if ts, ok := tv.(string); ok {
							tags[tk] = ts
						} else if ts, ok := tv.(bool); ok {
							tags[tk] = fmt.Sprintf("%v", ts)
						} else if ts, ok := tv.([]interface{}); ok {
							out, _ := json.Marshal(ts)
							tags[tk] = string(out)
						} else {
							if logger != nil {
								logger.Error("Invalid tags value type", zap.String("resourceType", resourceType), zap.Any("valueType", reflect.TypeOf(tv)), zap.Any("value", tv))
							}
							//return tags, "", fmt.Errorf("invalid tags value type: %s", reflect.TypeOf(tv))
						}
					}
				} else if tarr, ok := t.([]interface{}); ok {
					for _, tr := range tarr {
						if tmap, ok := tr.(map[string]string); ok {
							var key string
							for tk, tv := range tmap {
								if tk == "TagKey" {
									key = tv
								} else if tk == "TagValue" {
									tags[key] = tv
								}
							}
						} else if tmap, ok := tr.(map[string]interface{}); ok {
							var key string
							for tk, tv := range tmap {
								if ts, ok := tv.(string); ok {
									if tk == "TagKey" {
										key = ts
									} else if tk == "TagValue" {
										tags[key] = ts
									}
								} else {
									if logger != nil {
										logger.Error("Invalid tags js value type", zap.String("resourceType", resourceType), zap.Any("valueType", reflect.TypeOf(tv)), zap.Any("value", tv))
									}
									//return nil, "", fmt.Errorf("invalid tags js value type: %s", reflect.TypeOf(tv))
								}
							}
						}
					}
				} else {
					if logger != nil {
						logger.Error("Invalid tag type", zap.String("resourceType", resourceType), zap.Any("jsonBytes", string(jsonBytes)))
					}
					fmt.Printf("invalid tag type for: %s\n", string(jsonBytes))
					//return nil, "", fmt.Errorf("invalid tags type: %s", reflect.TypeOf(t))
				}
			}
		}
	}
	return tags, name, nil
}
