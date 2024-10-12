# csv
parse csv data to object

读取csv数据,转换成对象,支持自定义解析接口,以及结构嵌套(支持2层)

# 示例
```go
// 模拟一个物品配置结构
type ItemCfg struct {
  CfgId  int32  `json:"CfgId,omitempty"`  // 配置id
  Name   string `json:"Name,omitempty"`   // 物品名
  Detail string `json:"Detail,omitempty"` // 物品描述
  Unique bool   `json:"Unique,omitempty"` // 是否是不可叠加的物品
}
```
# csv数据转换成map
```go
rows := [][]string{
    {"CfgId", "Name", "Detail", "Unique", "unknownColumnTest"},
    {"1", "普通物品1", "普通物品1详细信息", "false", "123"},
    {"2", "普通物品2", "普通物品2详细信息", "false", "test"},
    {"3", "装备3", "装备3详细信息", "true", ""},
}
m := make(map[int32]*ItemCfg)
err := ReadCsvFromDataMap(rows, m, nil)
```

# csv数据转换成slice
```go
rows := [][]string{
    {"CfgId", "Name", "Detail", "Unique", "unknownColumnTest"},
    {"1", "普通物品1", "普通物品1详细信息", "false", "123"},
    {"2", "普通物品2", "普通物品2详细信息", "false", "test"},
    {"3", "装备3", "装备3详细信息", "true", ""},
}
s := make([]*ItemCfg, 0)
s,_ = ReadCsvFromDataSlice(rows, s, nil)
```

# key-value格式的csv数据转换成对象
```go
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
```

# 自定义解析接口
```go
rows := [][]string{
    {"CfgId", "Color", "ColorFlags"},
    {"1", "Red", "Red;Green;Blue"},
    {"2", "Gray", "Gray;Yellow"},
    {"3", "", ""},
}
type colorCfg struct {
    CfgId      int32
    Color      Color // Color是protobuf定义的枚举
    ColorFlags int32 // 颜色的组合值,如 Red | Green
}
option := DefaultCsvOption
// 注册颜色枚举的自定义解析接口,csv中可以直接填写颜色对应的字符串
option.RegisterConverterByType(reflect.TypeOf(Color(0)), func(obj any, columnName, fieldStr string) any {
    if colorValue, ok := Color_value["Color_"+fieldStr]; ok {
        return Color(colorValue)
    }
    return Color(0)
})
// 注册列名对应的解析接口
// 这里的ColorFlags列演示了一种特殊需求: 颜色的组合值用更易读的方式在csv中填写
option.RegisterConverterByColumnName("ColorFlags", func(obj any, columnName, fieldStr string) any {
    colorStrSlice := strings.Split(fieldStr, ";")
    flags := int32(0)
    for _, colorStr := range colorStrSlice {
        if colorValue, ok := Color_value["Color_"+colorStr]; ok && colorValue > 0 {
            flags |= 1 << (colorValue - 1)
        }
    }
    return flags
})
m := make(map[int]*colorCfg)
err := ReadCsvFromDataMap(rows, m, &option)
```

# 嵌套结构
详见csv_test.go里的TestNestStruct用例
