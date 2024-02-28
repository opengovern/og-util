package steampipe

import (
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"log"
	"net"
	"reflect"
	"strings"

	"github.com/golang/protobuf/ptypes"
	"github.com/hashicorp/go-hclog"
	"github.com/turbot/go-kit/helpers"
	"github.com/turbot/go-kit/types"
	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/context_key"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/transform"
)

func buildContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, context_key.Logger, hclog.New(nil))
	return ctx
}

func Cells(plg *plugin.Plugin, indexName string) ([]string, error) {
	var cells []string
	table, ok := plg.TableMap[indexName]
	if !ok {
		return cells, fmt.Errorf("invalid index name: %s", indexName)
	}
	table.Plugin = plg
	for _, column := range table.Columns {
		if column != nil && column.Transform != nil {
			cells = append(cells, column.Name)
		} else {
			fmt.Println("column or transform is null", column, column.Transform)
		}
	}

	return cells, nil
}

func DescriptionToRecord(logger *zap.Logger, plg *plugin.Plugin, resource interface{}, indexName string) (map[string]*proto.Column, error) {
	cells := make(map[string]*proto.Column)
	ctx := buildContext()
	table, ok := plg.TableMap[indexName]
	if !ok {
		if logger != nil {
			logger.Error("Invalid index name", zap.String("indexName", indexName), zap.Any("plugin", plg), zap.Any("resource", resource))
		}
		return cells, fmt.Errorf("invalid index name: %s", indexName)
	}
	table.Plugin = plg
	for _, column := range table.Columns {
		transformData := transform.TransformData{
			HydrateItem:    resource,
			HydrateResults: nil,
			ColumnName:     column.Name,
			KeyColumnQuals: nil,
		}

		if column != nil && column.Transform != nil {
			//value, err := column.Transform.Execute(ctx, &transformData, getDefaultColumnTransform(table, column))
			value, err := column.Transform.Execute(ctx, &transformData)
			if err != nil {
				if logger != nil {
					logger.Error("Error executing transform", zap.Error(err), zap.String("indexName", indexName), zap.String("columnName", column.Name), zap.Any("resource", resource))
				}
				return nil, err
			}

			c, err := interfaceToColumnValue(column, value)
			if err != nil {
				if logger != nil {
					logger.Error("Error converting to column value", zap.Error(err), zap.String("indexName", indexName), zap.String("columnName", column.Name), zap.Any("resource", resource))
				}
				return nil, err
			}

			cells[column.Name] = c
		} else if column == nil {
			//fmt.Println("column is null", indexName)
		} else if column.Transform == nil {
			//if indexName != "aws_cloudtrail_trail_event" && //ignore warnings
			//	indexName != "aws_cost_by_account_daily" &&
			//	indexName != "aws_ecr_repository" {
			//	fmt.Println("column transform is null", indexName, column.Name)
			//}
		}
	}

	return cells, nil
}

func getDefaultColumnTransform(t *plugin.Table, column *plugin.Column) *transform.ColumnTransforms {
	var columnTransform *transform.ColumnTransforms
	if defaultTransform := t.DefaultTransform; defaultTransform != nil {
		//did the table define a default transform
		columnTransform = defaultTransform
	} else if defaultTransform = t.Plugin.DefaultTransform; defaultTransform != nil {
		// maybe the plugin defined a default transform
		columnTransform = defaultTransform
	} else {
		// no table or plugin defined default transform - use the base default implementation
		// (just returning the field corresponding to the column name)
		columnTransform = &transform.ColumnTransforms{Transforms: []*transform.TransformCall{{Transform: transform.FieldValue, Param: column.Name}}}
	}
	return columnTransform
}

// convert a value of unknown type to a valid protobuf column value.type
func interfaceToColumnValue(column *plugin.Column, val interface{}) (*proto.Column, error) {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Errorf("%s: %v", column.Name, r))
		}
	}()

	// if the value is a pointer, get its value and use that
	val = helpers.DereferencePointer(val)
	if val == nil {
		if column.Default != nil {
			val = column.Default
		} else {
			// return nil
			return &proto.Column{Value: &proto.Column_NullValue{}}, nil
		}
	}

	var columnValue *proto.Column

	switch column.Type {
	case proto.ColumnType_STRING:
		columnValue = &proto.Column{Value: &proto.Column_StringValue{StringValue: types.ToString(val)}}
		break
	case proto.ColumnType_BOOL:
		b, err := types.ToBool(val)
		if err != nil {
			return nil, fmt.Errorf("interfaceToColumnValue failed for column '%s' with value '%v': %v", column.Name, val, err)
		}
		columnValue = &proto.Column{Value: &proto.Column_BoolValue{BoolValue: b}}
		break
	case proto.ColumnType_INT:
		i, err := types.ToInt64(val)
		if err != nil {
			return nil, fmt.Errorf("interfaceToColumnValue failed for column '%s' with value '%v': %v", column.Name, val, err)
		}

		columnValue = &proto.Column{Value: &proto.Column_IntValue{IntValue: i}}
		break
	case proto.ColumnType_DOUBLE:
		d, err := types.ToFloat64(val)
		if err != nil {
			return nil, fmt.Errorf("interfaceToColumnValue failed for column '%s' with value '%v': %v", column.Name, val, err)
		}
		columnValue = &proto.Column{Value: &proto.Column_DoubleValue{DoubleValue: d}}
		break
	case proto.ColumnType_JSON:
		strValue, ok := val.(string)
		if ok {
			// NOTE: Strings are assumed to be raw JSON, so are passed through directly.
			// This is the most common case, but means it's currently impossible to
			// pass through a string and have it marshalled to be a JSON representation
			// of a string.
			columnValue = &proto.Column{Value: &proto.Column_JsonValue{JsonValue: []byte(strValue)}}
		} else {
			res, err := json.Marshal(val)
			if err != nil {
				log.Printf("[ERROR] failed to marshal value to json: %v\n", err)
				return nil, fmt.Errorf("%s: %v ", column.Name, err)
			}
			columnValue = &proto.Column{Value: &proto.Column_JsonValue{JsonValue: res}}
		}
	case proto.ColumnType_DATETIME, proto.ColumnType_TIMESTAMP:
		// cast val to time
		if s, ok := val.(string); ok && s == "" {
			columnValue = &proto.Column{Value: &proto.Column_NullValue{}}
			break
		}
		var timeVal, err = types.ToTime(val)
		if err != nil {
			return nil, fmt.Errorf("interfaceToColumnValue failed for column '%s' with value '%v': %v", column.Name, val, err)
		}
		// now convert time to protobuf timestamp
		timestamp, err := ptypes.TimestampProto(timeVal)
		if err != nil {
			return nil, fmt.Errorf("interfaceToColumnValue failed for column '%s': %v", column.Name, err)
		}
		columnValue = &proto.Column{Value: &proto.Column_TimestampValue{TimestampValue: timestamp}}
		break
	case proto.ColumnType_IPADDR:
		ipString := types.SafeString(val)
		// treat an empty string as a null ip address
		if ipString == "" {
			columnValue = &proto.Column{Value: &proto.Column_NullValue{}}
		} else {
			if ip := net.ParseIP(ipString); ip == nil {
				return nil, fmt.Errorf("%s: invalid ip address %s", column.Name, ipString)
			}
			columnValue = &proto.Column{Value: &proto.Column_IpAddrValue{IpAddrValue: ipString}}
		}
		break
	case proto.ColumnType_CIDR:
		cidrRangeString := types.SafeString(val)
		// treat an empty string as a null ip address
		if cidrRangeString == "" {
			columnValue = &proto.Column{Value: &proto.Column_NullValue{}}
		} else {
			if _, _, err := net.ParseCIDR(cidrRangeString); err != nil {
				return nil, fmt.Errorf("%s: invalid cidr address %s", column.Name, cidrRangeString)
			}
			columnValue = &proto.Column{Value: &proto.Column_CidrRangeValue{CidrRangeValue: cidrRangeString}}
		}
		break
	default:
		return nil, fmt.Errorf("unrecognised columnValue type '%s'", column.Type)
	}

	return columnValue, nil

}

func ConvertToDescription(logger *zap.Logger, resourceType string, data interface{}, descriptionMap map[string]interface{}) (d interface{}, err error) {
	var b []byte
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger.Error("Error converting to description", zap.Any("resourceType", resourceType), zap.Any("data", data), zap.Any("descriptionMap", descriptionMap), zap.Any("recovered", r))
			}
			err = fmt.Errorf("paniced: %v\nresource_type: %s, json: %s", r, resourceType, string(b))
		}
	}()

	b, err = json.Marshal(data)
	if err != nil {
		if logger != nil {
			logger.Error("Error marshalling to description", zap.Error(err), zap.Any("resourceType", resourceType), zap.Any("data", data))
		}
		return nil, err
	}

	var dd any
	for k, v := range descriptionMap {
		if strings.ToLower(resourceType) == strings.ToLower(k) {
			dd = v
		}
	}
	d = reflect.New(reflect.ValueOf(dd).Type()).Interface()
	err = json.Unmarshal(b, &d)
	if err != nil {
		if logger != nil {
			logger.Error("Error unmarshalling to description", zap.Error(err), zap.Any("resourceType", resourceType), zap.Any("data", data))
		}
		return nil, fmt.Errorf("unmarshalling: %v", err)
	}

	d = helpers.DereferencePointer(d)
	return d, nil
}
