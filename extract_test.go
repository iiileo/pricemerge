package main

import (
	"testing"
)

func TestParseNamePrice(t *testing.T) {
	cases := []struct {
		raw   string
		name  string
		price float64
		ok    bool
	}{
		{"软牡丹133", "软牡丹", 133, true},
		{"软牡丹123-133", "软牡丹123", 133, true},
		{"123..133", "123", 133, true},
		{"软牡丹-129.5", "软牡丹", 129.5, true},
		{"硬中327", "硬中", 327, true},
		{"133", "", 0, false},
		{"软牡丹", "", 0, false},
		{"  软牡丹133  ", "软牡丹", 133, true},
		{"软牡丹１２３－１３３", "软牡丹123", 133, true},
		{"公主茶122文", "公主茶", 122, true},
		{"黑金圣63.5", "黑金圣", 63.5, true},
	}
	for _, c := range cases {
		got, ok := parseNamePrice(c.raw)
		if ok != c.ok {
			t.Fatalf("%q ok=%v want %v", c.raw, ok, c.ok)
		}
		if !c.ok {
			continue
		}
		if got.Name != c.name || got.Price != c.price {
			t.Fatalf("%q => %s/%v, want %s/%v", c.raw, got.Name, got.Price, c.name, c.price)
		}
	}
}

func TestExtractPairsFromText(t *testing.T) {
	pairs := extractPairsFromText("软牡丹133\n硬中327,芙蓉王206")
	if len(pairs) < 2 {
		t.Fatalf("got %#v", pairs)
	}
}

func TestExtractMultiLineAndFilterNoPrice(t *testing.T) {
	cell := "软牡丹133\n硬中\n芙蓉王206\n青柠牡丹"
	pairs := extractPairsFromText(cell)
	byName := map[string]float64{}
	for _, p := range pairs {
		byName[p.Name] = p.Price
	}
	if byName["软牡丹"] != 133 || byName["芙蓉王"] != 206 {
		t.Fatalf("%#v", pairs)
	}
	if _, ok := byName["硬中"]; ok {
		t.Fatalf("name without price should be filtered: %#v", pairs)
	}
	if _, ok := byName["青柠牡丹"]; ok {
		t.Fatalf("name without price should be filtered: %#v", pairs)
	}
}

func TestExtractSpaceSeparatedProducts(t *testing.T) {
	cell := "双中茶444        中支天叶茶905    蓝时代轿子茶67   泰山望月149\n" +
		"普细茶401        细天香茶407      朝天门茶64       软延安茶65\n" +
		"金细茶590        粗天叶685        缤果爆暂停       细好猫茶130\n" +
		"329..539         小目标123        x白轿子92        泰山茉莉茶暂停\n" +
		"黑金圣63.5       河北系列         黑兰州珍品152\n" +
		"新红茶  290\n金细  595\n公主茶122文\n普 细 4 1 0"
	pairs := extractPairsFromText(cell)
	by := map[string]float64{}
	for _, p := range pairs {
		by[p.Name] = p.Price
	}
	want := map[string]float64{
		"双中茶":   444,
		"中支天叶茶": 905,
		"普细茶":   401,
		"细天香茶":  407,
		"金细茶":   590,
		"粗天叶":   685,
		"细好猫茶":  130,
		"329":    539,
		"小目标":   123,
		"x白轿子":  92,
		"黑金圣":   63.5,
		"黑兰州珍品": 152,
		"新红茶":   290,
		"金细":    595,
		"公主茶":   122,
		"普细":    410,
	}
	for name, price := range want {
		if by[name] != price {
			t.Fatalf("%s want %v got %#v", name, price, pairs)
		}
	}
	for _, bad := range []string{"缤果爆暂停", "泰山茉莉茶暂停", "河北系列"} {
		if _, ok := by[bad]; ok {
			t.Fatalf("should filter %q", bad)
		}
	}
}

func TestExtractOnePerCellAndSameLineMulti(t *testing.T) {
	rows := [][]string{
		{"软牡丹133", "硬中327", "只有名"},
		{"软牡丹140 芙蓉王206"},
	}
	pairs := extractPairsFromRows(rows)
	byName := map[string]float64{}
	for _, p := range pairs {
		byName[p.Name] = p.Price
	}
	if byName["软牡丹"] != 140 { // later overrides
		t.Fatalf("软牡丹 override: %#v", pairs)
	}
	if byName["硬中"] != 327 || byName["芙蓉王"] != 206 {
		t.Fatalf("%#v", pairs)
	}
	if _, ok := byName["只有名"]; ok {
		t.Fatalf("filtered: %#v", pairs)
	}
}

func TestExtractTwoColumnRows(t *testing.T) {
	rows := [][]string{
		{"名称", "价格"},
		{"软牡丹", "133"},
		{"硬中", "327元"},
		{"芙蓉王123-206", "999"},
		{"开心就好143", "145"},
		{"无价格品", ""},
		{"", "100"},
		{"只有名"},
		{"金圣系列", "售价"},
		{"江苏系列", "售价"},
	}
	pairs := extractPairsFromRows(rows)
	byName := map[string]float64{}
	for _, p := range pairs {
		byName[p.Name] = p.Price
	}
	if byName["软牡丹"] != 133 {
		t.Fatalf("软牡丹: %#v", pairs)
	}
	if byName["硬中"] != 327 {
		t.Fatalf("硬中: %#v", pairs)
	}
	if byName["芙蓉王123-206"] != 999 {
		t.Fatalf("name+price cols keep name as-is: %#v", pairs)
	}
	if byName["开心就好143"] != 145 {
		t.Fatalf("embedded digits in name keep price col: %#v", pairs)
	}
	for _, bad := range []string{"无价格品", "只有名", "金圣系列", "江苏系列", "售价"} {
		if _, ok := byName[bad]; ok {
			t.Fatalf("should filter %q: %#v", bad, pairs)
		}
	}
}

func TestExtractMultiColumnNamePricePairs(t *testing.T) {
	rows := [][]string{
		{"名称", "售价", "名称", "售价"},
		{"软牡丹", "133", "硬中", "327"},
		{"芙蓉王", "206", "", ""},
		{"", "99", "青柠牡丹", "188"},
		{"只有名", "", "利群", "150"},
		{"中华", "450", "一格软兰州120"},
		{"山东系列", "售价", "公主", "125"},
	}
	pairs := extractPairsFromRows(rows)
	byName := map[string]float64{}
	for _, p := range pairs {
		byName[p.Name] = p.Price
	}
	want := map[string]float64{
		"软牡丹":   133,
		"硬中":    327,
		"芙蓉王":   206,
		"青柠牡丹":  188,
		"利群":    150,
		"中华":    450,
		"一格软兰州": 120,
		"公主":    125,
	}
	for name, price := range want {
		if byName[name] != price {
			t.Fatalf("%s want %v got %#v", name, price, pairs)
		}
	}
	for _, bad := range []string{"只有名", "山东系列", "售价"} {
		if _, ok := byName[bad]; ok {
			t.Fatalf("should filter %q: %#v", bad, pairs)
		}
	}
}
