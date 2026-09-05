package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseAndUpdateTemplate(t *testing.T) {
	path := "3-16模版数据公式.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip("template missing")
	}

	wb, err := openWorkbook(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.close()

	if len(wb.blocks) < 10 {
		t.Fatalf("expected many blocks, got %d", len(wb.blocks))
	}

	var hasOut, hasIn bool
	for _, b := range wb.blocks {
		if b.Kind == kindOut {
			hasOut = true
		}
		if b.Kind == kindIn {
			hasIn = true
		}
		t.Logf("block %s region=%q nameCol=%d priceCol=%d", b.Kind, b.Region, b.NameCol, b.PriceCol)
	}
	if !hasOut || !hasIn {
		t.Fatalf("missing 出/接 blocks: out=%v in=%v", hasOut, hasIn)
	}

	hits, err := wb.findByName(kindOut, "软牡丹")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hit for 软牡丹 出")
	}
	if hits[0].PriceCell != "AG7" || hits[0].DiffCell != "AH7" {
		t.Fatalf("expected AG7/AH7, got price=%s diff=%s", hits[0].PriceCell, hits[0].DiffCell)
	}
	t.Logf("hits: %+v", hits)

	porterBefore, _ := wb.file.GetCellValue(wb.sheet, "AF7")
	formulaBefore, _ := wb.file.GetCellFormula(wb.sheet, "AG7")
	if formulaBefore == "" {
		t.Fatal("expected 大琪 formula before update")
	}

	if _, err := wb.applyDaqiPrice(hits[0], 199.5); err != nil {
		t.Fatal(err)
	}

	porterAfter, _ := wb.file.GetCellValue(wb.sheet, "AF7")
	if porterBefore != porterAfter {
		t.Fatalf("搬运工 should not change: %q -> %q", porterBefore, porterAfter)
	}
	formulaAfter, _ := wb.file.GetCellFormula(wb.sheet, "AG7")
	if formulaAfter == "" {
		t.Fatal("大琪 formula was removed")
	}
	diffVal, _ := wb.file.GetCellValue(wb.sheet, "AH7")
	porterNum, _ := parseFloat(porterBefore)
	diffNum, ok := parseFloat(diffVal)
	if !ok || diffNum != porterNum-199.5 {
		t.Fatalf("差 should be %v, got %q", porterNum-199.5, diffVal)
	}

	out := filepath.Join(t.TempDir(), "out.xlsx")
	if err := wb.saveAs(out); err != nil {
		t.Fatal(err)
	}

	wb2, err := openWorkbook(out)
	if err != nil {
		t.Fatal(err)
	}
	defer wb2.close()

	f2, _ := wb2.file.GetCellFormula(wb2.sheet, "AG7")
	if f2 == "" {
		t.Fatal("大琪 formula missing after reload")
	}
	bFormula, _ := wb2.file.GetCellFormula(wb2.sheet, "B7")
	if bFormula == "" {
		t.Fatal("左侧 B7 引用公式丢失")
	}
	porterReload, _ := wb2.file.GetCellValue(wb2.sheet, "AF7")
	if porterReload != porterBefore {
		t.Fatalf("搬运工 changed after reload: %q -> %q", porterBefore, porterReload)
	}
	diffReload, _ := wb2.file.GetCellValue(wb2.sheet, "AH7")
	diffReloadNum, ok := parseFloat(diffReload)
	if !ok || diffReloadNum != porterNum-199.5 {
		t.Fatalf("差 after reload: %q", diffReload)
	}
}

func TestPriceServiceUpdateExport(t *testing.T) {
	s := &PriceService{}
	st, err := s.LoadDefaultTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Loaded || st.BlockCount == 0 {
		t.Fatalf("bad status: %+v", st)
	}

	res, err := s.UpdatePrice("软牡丹", "出", 188)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("update failed: %+v", res)
	}

	resIn, err := s.UpdatePrice("软牡丹", "接", 175)
	if err != nil {
		t.Fatal(err)
	}
	if !resIn.OK {
		t.Fatalf("update 接 failed: %+v", resIn)
	}

	out := filepath.Join(t.TempDir(), "exported.xlsx")
	path, err := s.Export(out)
	if err != nil {
		t.Fatal(err)
	}
	if path != out {
		t.Fatalf("path=%s", path)
	}

	wb, err := openWorkbook(out)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.close()

	// 有公式时：通过差反推大琪价，搬运工不变、公式保留
	outFormula, _ := wb.file.GetCellFormula(wb.sheet, "AG7")
	if outFormula == "" {
		t.Fatal("出 大琪公式丢失")
	}
	porter, _ := wb.file.GetCellValue(wb.sheet, "AF7")
	diff, _ := wb.file.GetCellValue(wb.sheet, "AH7")
	p, _ := parseFloat(porter)
	d, _ := parseFloat(diff)
	if p-d != 188 {
		t.Fatalf("出 大琪有效价应为 188, porter=%v diff=%v", p, d)
	}

	inFormula, _ := wb.file.GetCellFormula(wb.sheet, "AK7")
	if inFormula == "" {
		t.Fatal("接 大琪公式丢失")
	}
	porterIn, _ := wb.file.GetCellValue(wb.sheet, "AJ7")
	diffIn, _ := wb.file.GetCellValue(wb.sheet, "AL7")
	pi, _ := parseFloat(porterIn)
	di, _ := parseFloat(diffIn)
	if pi-di != 175 {
		t.Fatalf("接 大琪有效价应为 175, porter=%v diff=%v", pi, di)
	}
}

func TestMappingResolve(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "map.xlsx")
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "其他表名称")
	_ = f.SetCellValue("Sheet1", "B1", "基础表名称")
	_ = f.SetCellValue("Sheet1", "A2", "软牡丹别名")
	_ = f.SetCellValue("Sheet1", "B2", "软牡丹")
	if err := f.SaveAs(mapPath); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s := &PriceService{}
	if _, err := s.LoadDefaultTemplate(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadMapping(mapPath); err != nil {
		t.Fatal(err)
	}
	res, err := s.UpdatePrice("软牡丹别名", "出", 201)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.ResolvedName != "软牡丹" {
		t.Fatalf("mapping update: %+v", res)
	}
}
