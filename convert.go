package csv

import (
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

func ConvertCsvLineToValue(valueType reflect.Type, row []string, columnNames []string, option *CsvOption) reflect.Value {
	valueElemType := valueType
	if valueType.Kind() == reflect.Ptr {
		valueElemType = valueType.Elem() // *pb.ItemCfg -> pb.ItemCfg
	}
	newObject := reflect.New(valueElemType) // 如new(pb.ItemCfg)
	newObjectElem := newObject.Elem()
	if valueType.Kind() == reflect.Struct {
		newObject = newObject.Elem() // *pb.ItemCfg -> pb.ItemCfg
	}
	for columnIndex := 0; columnIndex < len(columnNames); columnIndex++ {
		columnName := columnNames[columnIndex]
		fieldString := row[columnIndex]
		fieldVal := newObjectElem.FieldByName(columnName)
		if fieldVal.Kind() == reflect.Ptr { // 指针类型的字段,如 Name *string
			fieldObj := reflect.New(fieldVal.Type().Elem()) // 如new(string)
			fieldVal.Set(fieldObj)                          // 如 obj.Name = new(string)
			fieldVal = fieldObj.Elem()                      // 如 *(obj.Name)
		}
		ConvertStringToFieldValue(newObject, fieldVal, columnName, fieldString, option, false)
	}
	return newObject
}

// 字段赋值,根据字段的类型,把字符串转换成对应的值
func ConvertStringToFieldValue(object, fieldVal reflect.Value, columnName, fieldString string, option *CsvOption, isSubStruct bool) {
	if !fieldVal.IsValid() {
		slog.Debug("unknown column", "columnName", columnName)
		return
	}
	if !fieldVal.CanSet() {
		slog.Error("field cant set", "columnName", columnName)
		return
	}
	var fieldConverter FieldConverter
	if !isSubStruct {
		fieldConverter = option.GetConverterByColumnName(columnName)
	}
	if fieldConverter != nil {
		// 列名注册的自定义的转换接口
		v := fieldConverter(object.Interface(), columnName, fieldString)
		fieldVal.Set(reflect.ValueOf(v))
	} else {
		var convertFieldToElem bool
		if !isSubStruct {
			fieldConverter, convertFieldToElem = option.GetConverterByTypePtrOrStruct(fieldVal.Type())
		}
		if fieldConverter != nil {
			// 类型注册的自定义的转换接口
			v := fieldConverter(object.Interface(), columnName, fieldString)
			if v == nil {
				slog.Debug("field parse error", "columnName", columnName, "fieldString", fieldString)
				return
			}
			if convertFieldToElem {
				fieldVal.Set(reflect.ValueOf(v).Elem())
			} else {
				fieldVal.Set(reflect.ValueOf(v))
			}
			return
		}
		// 常规类型
		switch fieldVal.Type().Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fieldVal.SetInt(Atoi64(fieldString))

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fieldVal.SetUint(Atou(fieldString))

		case reflect.String:
			fieldVal.SetString(fieldString)

		case reflect.Float32:
			f32, err := strconv.ParseFloat(fieldString, 32)
			if err != nil {
				slog.Error("float64 convert error", "columnName", columnName, "fieldString", fieldString, "err", err)
				break
			}
			fieldVal.SetFloat(f32)

		case reflect.Float64:
			f64, err := strconv.ParseFloat(fieldString, 64)
			if err != nil {
				slog.Error("float64 convert error", "columnName", columnName, "fieldString", fieldString, "err", err)
				break
			}
			fieldVal.SetFloat(f64)

		case reflect.Bool:
			fieldVal.SetBool(strings.ToLower(fieldString) == "true" || fieldString == "1")

		case reflect.Struct:
			if isSubStruct {
				// csv只是简单的以分隔符来解析,无法支持多层结构,子结构的字段名容易和注册的列名冲突,所以不支持嵌套多层结构体
				slog.Error("not support sub struct of sub struct", "columnName", columnName, "fieldString", fieldString)
				return
			}
			// 如CfgId_1#Num_2
			pairs := ParsePairString(fieldString, option)
			for _, pair := range pairs {
				subFieldVal := fieldVal.FieldByName(pair.Key)
				if !subFieldVal.IsValid() {
					slog.Error("fieldValue convert error", "columnName", columnName, "fieldString", fieldString, "fieldName", pair.Key, "fieldValue", pair.Value)
					continue
				}
				if subFieldVal.Kind() == reflect.Ptr { // 指针类型的字段,如 Name *string
					fieldObj := reflect.New(subFieldVal.Type().Elem()) // 如new(string)
					subFieldVal.Set(fieldObj)                          // 如 obj.Name = new(string)
					subFieldVal = fieldObj.Elem()                      // 如 *(obj.Name)
				}
				ConvertStringToFieldValue(fieldVal, subFieldVal, pair.Key, pair.Value, option, true)
			}

		case reflect.Slice:
			// 常规数组解析
			if fieldString == "" {
				return
			}
			newSlice := reflect.MakeSlice(fieldVal.Type(), 0, 0)
			sliceElemType := fieldVal.Type().Elem()
			converter, convertToElem := option.GetConverterByTypePtrOrStruct(sliceElemType)
			if converter == nil {
				if sliceElemType.Kind() == reflect.Struct {
					convertToElem = true
				} else if sliceElemType.Kind() == reflect.Ptr && sliceElemType.Elem().Kind() == reflect.Struct {
					sliceElemType = sliceElemType.Elem()
				}
			}
			sArray := strings.Split(fieldString, option.SliceSeparator)
			for _, str := range sArray {
				if str == "" {
					continue
				}
				var sliceElemValue any
				if converter != nil {
					sliceElemValue = converter(object.Interface(), columnName, str)
				} else {
					if sliceElemType.Kind() == reflect.Struct {
						fieldObj := reflect.New(sliceElemType) // 如obj := new(Struct)
						sliceElemValue = fieldObj.Interface()
						subFieldVal := fieldObj.Elem() // 如 *(obj)
						// 数组支持子结构
						ConvertStringToFieldValue(fieldVal, subFieldVal, "", str, option, isSubStruct)
					} else {
						sliceElemValue = ConvertStringToRealType(sliceElemType, str)
					}
				}
				if sliceElemValue == nil {
					slog.Error("slice item parse error", "columnName", columnName, "fieldString", fieldString, "str", str)
					continue
				}
				if convertToElem {
					newSlice = reflect.Append(newSlice, reflect.ValueOf(sliceElemValue).Elem())
				} else {
					newSlice = reflect.Append(newSlice, reflect.ValueOf(sliceElemValue))
				}
			}
			fieldVal.Set(newSlice)

		case reflect.Map:
			// 常规map解析
			if fieldString == "" {
				return
			}
			newMap := reflect.MakeMap(fieldVal.Type())
			fieldKeyType := fieldVal.Type().Key()
			fieldValueType := fieldVal.Type().Elem()
			converter, convertToElem := option.GetConverterByTypePtrOrStruct(fieldValueType)
			pairs := ParsePairString(fieldString, option)
			for _, pair := range pairs {
				fieldKeyValue := ConvertStringToRealType(fieldKeyType, pair.Key)
				var fieldValueValue any
				if converter != nil {
					fieldValueValue = converter(object.Interface(), columnName, pair.Value)
				} else {
					// NOTE: map不支持子结构,分隔符容易冲突
					fieldValueValue = ConvertStringToRealType(fieldValueType, pair.Value)
				}
				if fieldValueValue == nil {
					slog.Error("map value parse error", "columnName", columnName, "fieldString", fieldString, "pair", pair)
					continue
				}
				if convertToElem {
					newMap.SetMapIndex(reflect.ValueOf(fieldKeyValue), reflect.ValueOf(fieldValueValue).Elem())
				} else {
					newMap.SetMapIndex(reflect.ValueOf(fieldKeyValue), reflect.ValueOf(fieldValueValue))
				}
			}
			fieldVal.Set(newMap)

		default:
			slog.Error("unsupported kind", "columnName", columnName, "fieldVal", fieldVal, "kind", fieldVal.Type().Kind())
			return
		}
	}
}

type StringPair struct {
	Key   string
	Value string
}

// 把K1_V1#K2_V2#K3_V3转换成StringPair数组(如[{K1,V1},{K2,V2},{K3,V3}]
func convertPairString(pairs []*StringPair, cellString, pairSeparator, kvSeparator string) []*StringPair {
	pairSlice := strings.Split(cellString, pairSeparator)
	for _, pairString := range pairSlice {
		kv := strings.SplitN(pairString, kvSeparator, 2)
		if len(kv) != 2 {
			continue
		}
		pairs = append(pairs, &StringPair{
			Key:   kv[0],
			Value: kv[1],
		})
	}
	return pairs
}

// 把K1_V1#K2_V2#K3_V3转换成StringPair数组(如[{K1,V1},{K2,V2},{K3,V3}]
func ParsePairString(cellString string, option *CsvOption) []*StringPair {
	if option == nil {
		option = &DefaultOption
	}
	var pairs []*StringPair
	return convertPairString(pairs, cellString, option.PairSeparator, option.KvSeparator)
}

// 解析有嵌套结构的字符串
// 如 CfgId_1#ConsumeItems_{CfgId_1#Num_2;CfgId_2#Num_3}#Rewards_{CfgId_1#Num_1}#CountLimit_2
// 解析成 [{CfgId,1},{ConsumeItems,CfgId_1#Num_2;CfgId_2#Num_3},{Rewards,CfgId_1#Num_1},{CountLimit,2}]
func ParseNestString(cellString string, option *CsvOption, nestFieldNames ...string) []*StringPair {
	if option == nil {
		option = &DefaultOption
	}
	var pairs []*StringPair
	s := cellString
	for _, nestFieldName := range nestFieldNames {
		keyword := nestFieldName + option.KvSeparator + "{" // 如ConsumeItems_{
		beginPos := strings.Index(s, keyword)
		if beginPos >= 0 {
			endPos := strings.Index(s, "}")
			if endPos > beginPos {
				nestFieldValue := s[beginPos+len(keyword) : endPos]
				pairs = append(pairs, &StringPair{
					Key:   nestFieldName,
					Value: nestFieldValue,
				})
				if endPos < len(s)-2 {
					s = s[:beginPos] + s[endPos+1:]
				} else {
					s = s[:beginPos]
				}
			}
		}
	}
	return convertPairString(pairs, s, option.PairSeparator, option.KvSeparator)
}

// Name_a#Items_{CfgId_1#Num_1;CfgId_2#Num_1};Name_b#Items_{CfgId_1#Num_2;CfgId_2#Num_2}
func ParseNestStringSlice(cellString string, option *CsvOption, nestFieldNames ...string) [][]*StringPair {
	var pairsSlice [][]*StringPair
	idCounter := 0
	replaceKeys := make(map[int]*StringPair)
	s := cellString
	for _, nestFieldName := range nestFieldNames {
		for {
			keyword := nestFieldName + option.KvSeparator + "{" // 如Items_{
			beginPos := strings.Index(s, keyword)
			if beginPos < 0 {
				break
			}
			endPos := strings.Index(s, "}")
			if endPos > beginPos {
				nestFieldValue := s[beginPos+len(keyword) : endPos]
				idCounter++
				replaceKeys[idCounter] = &StringPair{
					Key:   nestFieldName,
					Value: nestFieldValue,
				}
				old := nestFieldName + option.KvSeparator + "{" + nestFieldValue + "}"
				// Items_{CfgId_1#Num_1;CfgId_2#Num_1}替换为Items_idCounter
				s = strings.Replace(s, old, nestFieldName+option.KvSeparator+strconv.Itoa(idCounter), 1)
			} else {
				break
			}
		}
	}
	// Name_a#Items_1;Name_b#Items_2
	elemSlice := strings.Split(s, option.SliceSeparator)
	for _, elem := range elemSlice {
		var pairs []*StringPair
		pairSlice := strings.Split(elem, option.PairSeparator)
		for _, pairString := range pairSlice {
			kv := strings.SplitN(pairString, option.KvSeparator, 2)
			if len(kv) != 2 {
				continue
			}
			if slices.Contains(nestFieldNames, kv[0]) {
				// 还原替换值
				id := Atoi(kv[1])
				if pair, ok := replaceKeys[id]; ok {
					pairs = append(pairs, pair)
				}
			} else {
				pairs = append(pairs, &StringPair{
					Key:   kv[0],
					Value: kv[1],
				})
			}
		}
		pairsSlice = append(pairsSlice, pairs)
	}
	return pairsSlice
}

func Atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

func Atoi64(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return i
}

func Atou(s string) uint64 {
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return u
}

// 支持int,float,string,[]byte,complex,bool
func ConvertStringToRealType(typ reflect.Type, s string) any {
	switch typ.Kind() {
	case reflect.Int:
		return Atoi(s)
	case reflect.Int8:
		return int8(Atoi(s))
	case reflect.Int16:
		return int16(Atoi(s))
	case reflect.Int32:
		return int32(Atoi(s))
	case reflect.Int64:
		return Atoi64(s)
	case reflect.Uint:
		return uint(Atou(s))
	case reflect.Uint8:
		return uint8(Atou(s))
	case reflect.Uint16:
		return uint16(Atou(s))
	case reflect.Uint32:
		return uint32(Atou(s))
	case reflect.Uint64:
		return Atou(s)
	case reflect.Float32:
		f, _ := strconv.ParseFloat(s, 32)
		return float32(f)
	case reflect.Float64:
		f, _ := strconv.ParseFloat(s, 64)
		return f
	case reflect.Complex64:
		c, _ := strconv.ParseComplex(s, 64)
		return c
	case reflect.Complex128:
		c, _ := strconv.ParseComplex(s, 128)
		return c
	case reflect.String:
		return s
	case reflect.Bool:
		return strings.ToLower(s) == "true" || s == "1"
	case reflect.Slice:
		// []byte
		if typ.Elem().Kind() == reflect.Uint8 {
			return []byte(s)
		}
	default:
		slog.Error("unsupported kind", "kind", typ.Kind())
	}
	return nil
}
