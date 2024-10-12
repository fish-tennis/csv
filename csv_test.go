package csv

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

// 模拟一个物品配置结构
type ItemCfg struct {
	CfgId  int32  `json:"CfgId,omitempty"`  // 配置id
	Name   string `json:"Name,omitempty"`   // 物品名
	Detail string `json:"Detail,omitempty"` // 物品描述
	Unique bool   `json:"Unique,omitempty"` // 是否是不可叠加的物品
}

// 物品Id和数量
type ItemNum struct {
	CfgId int32 `json:"CfgId,omitempty"` // 物品配置id
	Num   int32 `json:"Num,omitempty"`   // 物品数量
}

// 模拟一个protobuf定义的枚举
type Color int32

const (
	Color_Color_None   Color = 0
	Color_Color_Red    Color = 1
	Color_Color_Green  Color = 2
	Color_Color_Blue   Color = 3
	Color_Color_Yellow Color = 4
	Color_Color_Gray   Color = 5
)

// Enum value maps for Color.
var (
	Color_name = map[int32]string{
		0: "Color_None",
		1: "Color_Red",
		2: "Color_Green",
		3: "Color_Blue",
		4: "Color_Yellow",
		5: "Color_Gray",
	}
	Color_value = map[string]int32{
		"Color_None":   0,
		"Color_Red":    1,
		"Color_Green":  2,
		"Color_Blue":   3,
		"Color_Yellow": 4,
		"Color_Gray":   5,
	}
)

func TestReadCsvFromDataProto(t *testing.T) {
	rows := [][]string{
		{"CfgId", "Name", "Detail", "Unique", "unknownColumnTest"},
		{"1", "普通物品1", "普通物品1详细信息", "false", "123"},
		{"2", "普通物品2", "普通物品2详细信息", "false", "test"},
		{"3", "装备3", "装备3详细信息", "true", ""},
	}
	m := make(map[int32]*ItemCfg)
	err := ReadCsvFromDataMap(rows, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range m {
		t.Logf("%v", item)
	}
}

func TestReadCsvFromDataStruct(t *testing.T) {
	rows := [][]string{
		{"Name", "Detail", "Unique", "SliceTest", "MapTest"},
		{"item1", "普通物品1详细信息", "false", "1;2;3", "a_1#b_2#c_3"},
		{"item2", "普通物品2详细信息", "false", "4", "d_4"},
		{"item3", "装备3详细信息", "true", "", ""},
	}
	// 测试非proto.Message的map格式
	type testItemCfg struct {
		Name      string
		Detail    *string // 测试指针类型的字段
		Unique    bool
		SliceTest []int
		MapTest   map[string]int32
	}
	// map的key也可以是字符串
	m := make(map[string]testItemCfg)
	err := ReadCsvFromDataMap(rows, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range m {
		t.Logf("%v", item)
		t.Logf("Detail:%v", *item.Detail)
	}
}

func TestReadCsvFromDataConverter(t *testing.T) {
	rows := [][]string{
		{"CfgId", "Name", "Item", "Items", "ColorFlags", "Color", "ColorPtr", "ItemStruct", "ItemStructs", "ItemMap"},
		{"1", "Name1", "123_1", "123_1;456_2", "Red;Green;Blue", "Red", "Red", "123_1", "321_1;654_2", "1_321_1#2_654_2"},
		{"2", "Name2", "456_5", "123_1", "Gray;Yellow", "Gray", "Gray", "456_5", "321_1", "1_321_1"},
		{"3", "Name3", "789_10", "", "", "", "", "", "", ""},
	}
	type testCfg struct {
		CfgId      int32
		Name       string
		Item       *ItemNum
		ItemStruct ItemNum // 注册了&pb.ItemNum{}接口,ItemNum也会被正确解析

		Items       []*ItemNum
		ItemStructs []ItemNum // 注册了&pb.ItemNum{}接口,[]ItemNum也会被正确解析

		ItemMap map[int32]*ItemNum

		Color      Color
		ColorPtr   *Color // 注册了Color接口,*Color也会被正确解析
		ColorFlags int32  // 颜色的组合值,如 Red | Green
	}

	option := DefaultOption

	// 注册pb.ItemNum的解析接口
	option.RegisterConverterByType(reflect.TypeOf(&ItemNum{}), func(obj interface{}, columnName, fieldStr string) interface{} {
		strs := strings.Split(fieldStr, "_")
		if len(strs) != 2 {
			return nil
		}
		return &ItemNum{
			CfgId: int32(Atoi(strs[0])),
			Num:   int32(Atoi(strs[1])),
		}
	})
	// 注册颜色枚举的自定义解析接口,csv中可以直接填写颜色对应的字符串
	option.RegisterConverterByType(reflect.TypeOf(Color(0)), func(obj interface{}, columnName, fieldStr string) interface{} {
		t.Logf("pb.Color parse columnName:%v,fieldStr:%v", columnName, fieldStr)
		if colorValue, ok := Color_value["Color_"+fieldStr]; ok {
			return Color(colorValue)
		}
		return Color(0)
	})
	// 注册列名对应的解析接口
	// 这里的ColorFlags列演示了一种特殊需求: 颜色的组合值用更易读的方式在csv中填写
	option.RegisterConverterByColumnName("ColorFlags", func(obj interface{}, columnName, fieldStr string) interface{} {
		colorStrs := strings.Split(fieldStr, ";")
		flags := int32(0)
		for _, colorStr := range colorStrs {
			if colorValue, ok := Color_value["Color_"+colorStr]; ok && colorValue > 0 {
				flags |= 1 << (colorValue - 1)
			}
		}
		t.Logf("ColorFlags parse columnName:%v,fieldStr:%v flags:%v", columnName, fieldStr, flags)
		return flags
	})

	m := make(map[int]*testCfg)
	err := ReadCsvFromDataMap(rows, m, &option)
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range m {
		t.Logf("%v", cfg)
		t.Logf("%v", cfg.Item)
		t.Logf("%v", &cfg.ItemStruct)
		for i, item := range cfg.Items {
			t.Logf("Items[%v]:%v", i, item)
		}
		t.Logf("%v", cfg.ItemStructs)
		for k, item := range cfg.ItemMap {
			t.Logf("ItemMap[%v]:%v", k, item)
		}
	}
}

func TestCustomConverter(t *testing.T) {
	rows := [][]string{
		{"CfgId", "Color", "ColorFlags"},
		{"1", "Red", "Red;Green;Blue"},
		{"2", "Gray", "Gray;Yellow"},
		{"3", "", ""},
	}
	type colorCfg struct {
		CfgId      int32
		Color      Color
		ColorFlags int32 // 颜色的组合值,如 Red | Green
	}
	option := DefaultOption
	// 注册颜色枚举的自定义解析接口,csv中可以直接填写颜色对应的字符串
	option.RegisterConverterByType(reflect.TypeOf(Color(0)), func(obj interface{}, columnName, fieldStr string) interface{} {
		t.Logf("pb.Color parse columnName:%v,fieldStr:%v", columnName, fieldStr)
		if colorValue, ok := Color_value["Color_"+fieldStr]; ok {
			return Color(colorValue)
		}
		return Color(0)
	})
	// 注册列名对应的解析接口
	// 这里的ColorFlags列演示了一种特殊需求: 颜色的组合值用更易读的方式在csv中填写
	option.RegisterConverterByColumnName("ColorFlags", func(obj interface{}, columnName, fieldStr string) interface{} {
		colorStrSlice := strings.Split(fieldStr, ";")
		flags := int32(0)
		for _, colorStr := range colorStrSlice {
			if colorValue, ok := Color_value["Color_"+colorStr]; ok && colorValue > 0 {
				flags |= 1 << (colorValue - 1)
			}
		}
		t.Logf("ColorFlags parse columnName:%v,fieldStr:%v flags:%v", columnName, fieldStr, flags)
		return flags
	})

	m := make(map[int]*colorCfg)
	err := ReadCsvFromDataMap(rows, m, &option)
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range m {
		t.Logf("%v", cfg)
	}
}

func TestMapReflect(t *testing.T) {
	type testStruct struct {
		I int
		S string
	}
	m := make(map[int]testStruct)
	mType := reflect.TypeOf(m)
	mVal := reflect.ValueOf(m)
	//keyType := mType.Key()    // int
	valueType := mType.Elem() // pb.ItemCfg
	t.Logf("%v", valueType)
	key := 1
	newItem := reflect.New(valueType) // new(pb.ItemCfg)
	newItem.Elem().FieldByName("I").SetInt(123)
	newItem.Elem().FieldByName("S").SetString("abc")
	mVal.SetMapIndex(reflect.ValueOf(key), newItem.Elem())
	for _, cfg := range m {
		t.Logf("%v", cfg)
	}
}

func TestReadCsvFromDataSlice(t *testing.T) {
	rows := [][]string{
		{"CfgId", "Name", "Detail", "Unique", "unknownColumnTest"},
		{"1", "普通物品1", "普通物品1详细信息", "false", "123"},
		{"2", "普通物品2", "普通物品2详细信息", "false", "test"},
		{"3", "装备3", "装备3详细信息", "true", ""},
	}
	s := make([]*ItemCfg, 0)
	newSlice, err := ReadCsvFromDataSlice(rows, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range newSlice {
		t.Logf("%v: %v", i, item)
	}
}

func TestReadCsvFromDataObject(t *testing.T) {
	type Settings struct {
		ImageQuality int
		Volume       int
		Language     string
	}
	rows := [][]string{
		{"Key", "Value", "comment"},
		{"ImageQuality", "100", "画质设置"},
		{"Volume", "80", "音量设置"},
		{"Language", "Simplified Chinese", "语言设置"},
	}
	settings := new(Settings)
	err := ReadCsvFromDataObject(rows, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", settings)
}

func TestParseNestString(t *testing.T) {
	s := "CfgId_1#ConsumeItems_{CfgId_1#Num_2;CfgId_2#Num_3}#Rewards_{CfgId_1#Num_1}#CountLimit_2"
	pairs := ParseNestString(s, DefaultOption.PairSeparator, DefaultOption.KvSeparator, "ConsumeItems", "Rewards")
	for _, pair := range pairs {
		t.Logf("%v %v", pair.Key, pair.Value)
	}
}

func TestNestStruct(t *testing.T) {
	type Child struct {
		Name  string
		Items []*ItemNum // 物品列表
	}
	type cfg struct {
		CfgId    int32
		Children []*Child // 子对象列表
	}
	rows := [][]string{
		{"CfgId", "Children"},
		{"1", "Name_a#Items_{CfgId_1#Num_1;CfgId_2#Num_1};Name_b#Items_{CfgId_1#Num_2;CfgId_2#Num_2}"},
		{"2", "Name_c#Items_{CfgId_3#Num_1;CfgId_4#Num_2}"},
	}
	option := DefaultOption
	// 嵌套结构需要注册自定义接口来解析子结构
	var childrenType []*Child
	option.RegisterConverterByType(reflect.TypeOf(childrenType), func(obj any, columnName, fieldStr string) any {
		newChildren := make([]*Child, 0)
		// Name_a#Items_{CfgId_1#Num_1;CfgId_2#Num_1};Name_b#Items_{CfgId_1#Num_2;CfgId_2#Num_2}
		pairsSlice := ParseNestStringSlice(fieldStr, option.PairSeparator, option.KvSeparator, "Items")
		for _, pairs := range pairsSlice {
			child := &Child{}
			objVal := reflect.ValueOf(child).Elem()
			for _, pair := range pairs {
				t.Logf("%v %v", pair.Key, pair.Value)
				fieldVal := objVal.FieldByName(pair.Key)
				ConvertStringToFieldValue(objVal, fieldVal, pair.Key, pair.Value, &option, false)
			}
			newChildren = append(newChildren, child)
		}
		return newChildren
	})
	s := make([]*cfg, 0)
	s, err := ReadCsvFromDataSlice(rows, s, &option)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range s {
		t.Logf("CfgId:%v", v.CfgId)
		for i, child := range v.Children {
			t.Logf("Children[%v].Name:%v", i, child.Name)
			for j, item := range child.Items {
				t.Logf("Children[%v].Items[%v]:%v", i, j, item)
			}
		}
	}
}
