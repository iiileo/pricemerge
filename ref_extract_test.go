package main

import (
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func loadSheetPairs(t *testing.T, path string) []extractedPair {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skip(path + " missing")
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetList()[0])
	if err != nil {
		t.Fatal(err)
	}
	return extractPairsFromRows(rows)
}

func TestRefSheetNamePriceColumns(t *testing.T) {
	pairs := loadSheetPairs(t, "数据表.xlsx")
	by := map[string]float64{}
	for _, p := range pairs {
		by[p.Name] = p.Price
	}
	if len(pairs) < 200 {
		t.Fatalf("expected many pairs, got %d", len(pairs))
	}
	checks := map[string]float64{
		"国瓷":   690,
		"休闲云端": 855,
		"软牡丹":  133,
		"公主":   125,
		"开心就好143": 145,
	}
	for name, price := range checks {
		if by[name] != price {
			t.Fatalf("%s want %v got %v", name, price, by[name])
		}
	}
	for _, bad := range []string{"金圣系列", "江苏系列", "山东系列", "售价", "金圣金中支"} {
		if _, ok := by[bad]; ok {
			t.Fatalf("should skip %q", bad)
		}
	}
}

func TestRefSheetPackedCells(t *testing.T) {
	pairs := loadSheetPairs(t, "数据表1.xlsx")
	by := map[string]float64{}
	for _, p := range pairs {
		by[p.Name] = p.Price
	}
	if len(pairs) < 100 {
		t.Fatalf("expected many pairs, got %d", len(pairs))
	}
	checks := map[string]float64{
		"中支王茶": 250,
		"牡丹":   133,
		"软1916": 790,
		"1906..茶": 150,
		"微商茶":  315,
		"普皖茶":  128,
	}
	for name, price := range checks {
		if by[name] != price {
			t.Fatalf("%s want %v got %v", name, price, by[name])
		}
	}
	for _, bad := range []string{"芙蓉王茶系列", "金圣系列", "领航茶", "短王茶"} {
		if _, ok := by[bad]; ok {
			t.Fatalf("should skip %q", bad)
		}
	}
	for name := range by {
		if strings.HasSuffix(name, "系列") {
			t.Fatalf("series header leaked: %q", name)
		}
	}
}
