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
		{"无价格品", ""},
		{"", "100"},
		{"只有名"},
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
	if byName["芙蓉王123"] != 206 {
		t.Fatalf("embedded: %#v", pairs)
	}
	if _, ok := byName["无价格品"]; ok {
		t.Fatalf("empty price should filter: %#v", pairs)
	}
	if _, ok := byName["只有名"]; ok {
		t.Fatalf("name-only should filter: %#v", pairs)
	}
}
