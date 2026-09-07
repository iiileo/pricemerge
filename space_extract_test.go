package main

import (
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestDataSheet3SpaceSeparated(t *testing.T) {
	if _, err := os.Stat("数据表3.xlsx"); err != nil {
		t.Skip("missing")
	}
	f, err := excelize.OpenFile("数据表3.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.GetRows(f.GetSheetList()[0])
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	var big string
	for _, row := range rows {
		for _, c := range row {
			if strings.Contains(c, "双中茶444") {
				big = c
			}
		}
	}
	if big == "" {
		t.Fatal("space-separated cell not found")
	}
	fromBig := extractPairsFromText(big)
	byBig := map[string]float64{}
	for _, p := range fromBig {
		byBig[p.Name] = p.Price
	}
	wantBig := map[string]float64{
		"双中茶":   444,
		"中支天叶茶": 905,
		"普细茶":   401,
		"黑金圣":   63.5,
		"公主茶":   122,
		"软金圣茶":  73,
		"招财猫茶":  135,
	}
	for k, v := range wantBig {
		if byBig[k] != v {
			t.Errorf("big cell %s want %v got %v", k, v, byBig[k])
		}
	}
	if _, ok := byBig["缤果爆暂停"]; ok {
		t.Error("should skip 暂停 items")
	}
	if len(fromBig) < 40 {
		t.Fatalf("expected many products from big cell, got %d", len(fromBig))
	}

	all := extractPairsFromRows(rows)
	by := map[string]float64{}
	for _, p := range all {
		by[p.Name] = p.Price
	}
	// unique-ish values that should survive whole-sheet merge
	if by["双中茶"] != 444 {
		t.Errorf("双中茶=%v", by["双中茶"])
	}
	if by["金细"] != 595 {
		t.Errorf("金细=%v", by["金细"])
	}
	if by["黑金圣"] != 63.5 {
		t.Errorf("黑金圣=%v", by["黑金圣"])
	}
	if len(all) < 350 {
		t.Fatalf("expected dense extract, got %d", len(all))
	}
}
