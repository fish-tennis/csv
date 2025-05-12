package csv

import (
	"encoding/csv"
	"errors"
	"os"
	"reflect"
	"slices"
)

// 默认csv设置
var DefaultOption = CsvOption{
	ColumnNameRowIndex: 0,
	DataBeginRowIndex:  1, // csv行索引
	SliceSeparator:     ";",
	KvSeparator:        "_",
	PairSeparator:      "#",
}

// 字段转换接口
type FieldConverter func(obj any, columnName, fieldStr string) any

type CsvOption struct {
	// 数据行索引(>=1)
	DataBeginRowIndex int

	// 字段名数据行索引(>=0)
	ColumnNameRowIndex int

	// key-value格式的csv数据给对象赋值,数据行索引(>=0)
	ObjectDataBeginRowIndex int

	// 是否禁用protobuf的字段别名(struct tag里的name),默认不禁用
	//  proto2示例 Num *int32 `protobuf:"varint,1,opt,name=num"`
	//  proto3示例 Num int32  `protobuf:"varint,1,opt,name=num,proto3"`
	DisableProtobufAliasName bool

	// 是否禁用json的字段别名(struct tag里的name),默认不禁用
	//  示例 Num *int32 `json:"num,omitempty"`
	DisableJsonAliasName bool

	// 数组分隔符
	// 如数组分隔符为;时,则1;2;3可以表示[1,2,3]的数组
	SliceSeparator string

	// Key-Value分隔符
	// 如KvSeparator为_ PairSeparator为#
	// 则a_1#b_2#c_3可以表示{"a":1,"b":2,"c":3}的map或者如下结构体
	// type S struct {
	//   a string
	//   b string
	//   c int
	// }
	KvSeparator string

	// 不同Key-Value之间的分隔符
	// 如KvSeparator为_ PairSeparator为#
	// 则a_1#b_2#c_3可以表示{"a":1,"b":2,"c":3}的map或者如下结构体
	// type S struct {
	//   a string
	//   b string
	//   c int
	// }
	PairSeparator string

	// 自定义转换函数
	// 把csv的字符串转换成其他对象 以列名作为关键字
	customFieldConvertersByColumnName map[string]FieldConverter
	// 把csv的字符串转换成其他对象 以字段类型作为关键字
	customFieldConvertersByType map[reflect.Type]FieldConverter

	// 忽略的列名,如单纯的注释列
	ignoreColumns map[string]struct{}
}

// 注册列名对应的转换接口
func (co *CsvOption) RegisterConverterByColumnName(columnName string, converter FieldConverter) *CsvOption {
	if co.customFieldConvertersByColumnName == nil {
		co.customFieldConvertersByColumnName = make(map[string]FieldConverter)
	}
	co.customFieldConvertersByColumnName[columnName] = converter
	return co
}

func (co *CsvOption) GetConverterByColumnName(columnName string) FieldConverter {
	if co.customFieldConvertersByColumnName == nil {
		return nil
	}
	return co.customFieldConvertersByColumnName[columnName]
}

// 注册类型对应的转换接口
func (co *CsvOption) RegisterConverterByType(typ reflect.Type, converter FieldConverter) *CsvOption {
	if co.customFieldConvertersByType == nil {
		co.customFieldConvertersByType = make(map[reflect.Type]FieldConverter)
	}
	co.customFieldConvertersByType[typ] = converter
	return co
}

func (co *CsvOption) GetConverterByType(typ reflect.Type) FieldConverter {
	if co.customFieldConvertersByType == nil {
		return nil
	}
	return co.customFieldConvertersByType[typ]
}

// 如果typ是Struct,但是注册的FieldConverter是同类型的Ptr,则会返回Ptr类型的FieldConverter,同时convertToElem返回true
func (co *CsvOption) GetConverterByTypePtrOrStruct(typ reflect.Type) (converter FieldConverter, convertToElem bool) {
	if co.customFieldConvertersByType == nil {
		return
	}
	converter, _ = co.customFieldConvertersByType[typ]
	if converter == nil {
		if typ.Kind() == reflect.Struct {
			converter = co.GetConverterByType(reflect.PointerTo(typ))
			// 注册的是指针类型,转换后,需要把ptr转换成elem
			convertToElem = converter != nil
			return
		}
	}
	return
}

// 设置需要忽略的列名,如单纯的注释列
func (co *CsvOption) IgnoreColumn(columnNames ...string) {
	if co.ignoreColumns == nil {
		co.ignoreColumns = make(map[string]struct{})
	}
	for _, columnName := range columnNames {
		co.ignoreColumns[columnName] = struct{}{}
	}
}

type IntOrString interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~string
}

// csv数据转换成map
// V支持proto.Message和普通struct结构
func ReadCsvFileMap[M ~map[K]V, K IntOrString, V any](file string, m M, option *CsvOption) error {
	rows, readErr := ReadCsvFile(file)
	if readErr != nil {
		return readErr
	}
	return ReadCsvFromDataMap(rows, m, option)
}

// csv数据转换成slice
// V支持proto.Message和普通struct结构
func ReadCsvFileSlice[Slice ~[]V, V any](file string, s Slice, option *CsvOption) (Slice, error) {
	rows, readErr := ReadCsvFile(file)
	if readErr != nil {
		return s, readErr
	}
	return ReadCsvFromDataSlice(rows, s, option)
}

// key-value格式的csv数据给对象赋值
// V支持proto.Message和普通struct结构
func ReadCsvFileObject[V any](file string, v V, option *CsvOption) error {
	rows, readErr := ReadCsvFile(file)
	if readErr != nil {
		return readErr
	}
	return ReadCsvFromDataObject(rows, v, option)
}

func ReadCsvFile(file string) ([][]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}

// csv数据转换成map
// V支持proto.Message和普通struct结构
func ReadCsvFromDataMap[M ~map[K]V, K IntOrString, V any](rows [][]string, m M, option *CsvOption) error {
	if option == nil {
		option = &DefaultOption
	}
	if len(rows) == 0 {
		return errors.New("no csv header")
	}
	if len(rows) <= option.ColumnNameRowIndex {
		return errors.New("no column name header")
	}
	columnNames := rows[option.ColumnNameRowIndex]
	if len(columnNames) == 0 {
		return errors.New("no column")
	}
	if option.DataBeginRowIndex < 1 {
		return errors.New("DataBeginRowIndex must >=1")
	}
	mType := reflect.TypeOf(m)
	mVal := reflect.ValueOf(m)
	keyType := mType.Key()    // key type of m, 如int
	valueType := mType.Elem() // value type of m, 如*pb.ItemCfg or pb.ItemCfg
	for rowIndex := option.DataBeginRowIndex; rowIndex < len(rows); rowIndex++ {
		row := rows[rowIndex]
		// 固定第一列是key
		key := ConvertStringToRealType(keyType, row[0])
		value := ConvertCsvLineToValue(valueType, row, columnNames, option)
		mVal.SetMapIndex(reflect.ValueOf(key), value)
	}
	return nil
}

// csv数据转换成slice
// V支持proto.Message和普通struct结构
func ReadCsvFromDataSlice[Slice ~[]V, V any](rows [][]string, s Slice, option *CsvOption) (Slice, error) {
	if option == nil {
		option = &DefaultOption
	}
	if len(rows) == 0 {
		return s, errors.New("no csv header")
	}
	if len(rows) <= option.ColumnNameRowIndex {
		return s, errors.New("no column name header")
	}
	columnNames := rows[option.ColumnNameRowIndex]
	if len(columnNames) == 0 {
		return s, errors.New("no column")
	}
	if option.DataBeginRowIndex < 1 {
		return s, errors.New("DataBeginRowIndex must >=1")
	}
	sType := reflect.TypeOf(s)
	valueType := sType.Elem() // value type of s, 如*pb.ItemCfg or pb.ItemCfg
	for rowIndex := option.DataBeginRowIndex; rowIndex < len(rows); rowIndex++ {
		row := rows[rowIndex]
		value := ConvertCsvLineToValue(valueType, row, columnNames, option)
		s = slices.Insert(s, len(s), value.Interface().(V)) // s = append(s, value)
	}
	return s, nil
}

// key-value格式的csv数据转换成对象
// V支持proto.Message和普通struct结构
func ReadCsvFromDataObject[V any](rows [][]string, v V, option *CsvOption) error {
	if option == nil {
		option = &DefaultOption
	}
	if len(rows) == 0 {
		return errors.New("no csv header")
	}
	if len(rows[0]) < 2 {
		return errors.New("column count must >= 2")
	}
	if option.ObjectDataBeginRowIndex < 1 {
		return errors.New("ObjectDataBeginRowIndex must >=1")
	}
	typ := reflect.TypeOf(v) // type of v, 如*pb.ItemCfg or pb.ItemCfg
	val := reflect.ValueOf(v)
	if typ.Kind() != reflect.Ptr {
		return errors.New("v must be Ptr")
	}
	valElem := val.Elem() // *pb.ItemCfg -> pb.ItemCfg
	// protobuf alias name map
	var aliasNames map[string]string
	for rowIndex := option.ObjectDataBeginRowIndex; rowIndex < len(rows); rowIndex++ {
		row := rows[rowIndex]
		// key-value的固定格式,列名不用
		columnName := row[0]
		fieldString := row[1]
		fieldVal := valElem.FieldByName(columnName)
		if !fieldVal.IsValid() {
			if aliasNames == nil {
				aliasNames = getAliasNameMap(valElem.Type(), option)
			}
			// xxx.proto里定义的字段名可能是cfg_id
			// 生成的xxx.pb里面的字段名会变成CfgId
			// 如果csv里面的列名使用cfg_id也要能解析
			if realFieldName, ok := aliasNames[columnName]; ok {
				fieldVal = valElem.FieldByName(realFieldName)
			}
		}
		if fieldVal.Kind() == reflect.Ptr { // 指针类型的字段,如 Name *string
			fieldObj := reflect.New(fieldVal.Type().Elem()) // 如new(string)
			fieldVal.Set(fieldObj)                          // 如 obj.Name = new(string)
			fieldVal = fieldObj.Elem()                      // 如 *(obj.Name)
		}
		ConvertStringToFieldValue(val, fieldVal, columnName, fieldString, option, false)
	}
	return nil
}
