package main

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestPreviewAndApplyBatch(t *testing.T) {
	dir := t.TempDir()
	pricePath := filepath.Join(dir, "prices.xlsx")
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "软牡丹133")
	_ = f.SetCellValue("Sheet1", "B1", "硬中327")
	_ = f.SetCellValue("Sheet1", "A2", "不存在的烟999")
	if err := f.SaveAs(pricePath); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s := &PriceService{}
	if _, err := s.LoadDefaultTemplate(); err != nil {
		t.Fatal(err)
	}
	prev, err := s.PreviewPriceSheet(pricePath, "出")
	if err != nil {
		t.Fatal(err)
	}
	if !prev.OK || len(prev.Items) < 2 {
		t.Fatalf("preview: %#v", prev)
	}
	// preview must not dirty workbook
	if s.Status().Dirty {
		t.Fatal("preview should not dirty")
	}

	applied, err := s.ApplyPendingBatch()
	if err != nil {
		t.Fatal(err)
	}
	if applied.Updated < 1 || applied.Missed < 1 {
		t.Fatalf("apply: %#v", applied)
	}
	hasChange := false
	for _, r := range applied.Rows {
		if r.Status == "updated" && r.Name == "软牡丹" {
			hasChange = true
			if r.NewPrice != 133 {
				t.Fatalf("row: %#v", r)
			}
		}
	}
	if !hasChange {
		t.Fatalf("missing 软牡丹 row: %#v", applied.Rows)
	}
}

func TestImportPriceSheet(t *testing.T) {
	dir := t.TempDir()
	pricePath := filepath.Join(dir, "prices.xlsx")
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "软牡丹133")
	_ = f.SetCellValue("Sheet1", "B1", "硬中327")
	_ = f.SetCellValue("Sheet1", "A2", "不存在的烟999")
	if err := f.SaveAs(pricePath); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s := &PriceService{}
	if _, err := s.LoadDefaultTemplate(); err != nil {
		t.Fatal(err)
	}
	res, err := s.ImportPriceSheet(pricePath, "出")
	if err != nil {
		t.Fatal(err)
	}
	if res.Extracted < 2 {
		t.Fatalf("extracted=%d %#v", res.Extracted, res)
	}
	if res.Updated < 1 {
		t.Fatalf("expected updates: %#v", res)
	}
	if res.Missed < 1 {
		t.Fatalf("expected misses: %#v", res)
	}
}

func TestImportTwoColumnPriceSheet(t *testing.T) {
	dir := t.TempDir()
	pricePath := filepath.Join(dir, "two_col.xlsx")
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "名称")
	_ = f.SetCellValue("Sheet1", "B1", "价格")
	_ = f.SetCellValue("Sheet1", "A2", "软牡丹")
	_ = f.SetCellValue("Sheet1", "B2", "188")
	_ = f.SetCellValue("Sheet1", "A3", "硬中")
	_ = f.SetCellValue("Sheet1", "B3", "300")
	if err := f.SaveAs(pricePath); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s := &PriceService{}
	if _, err := s.LoadDefaultTemplate(); err != nil {
		t.Fatal(err)
	}
	res, err := s.ImportPriceSheet(pricePath, "出")
	if err != nil {
		t.Fatal(err)
	}
	if res.Extracted != 2 || res.Updated != 2 {
		t.Fatalf("two-col import: %#v", res)
	}
}

func TestUndoBatchRowAndAll(t *testing.T) {
	dir := t.TempDir()
	pricePath := filepath.Join(dir, "prices.xlsx")
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "名称")
	_ = f.SetCellValue("Sheet1", "B1", "价格")
	_ = f.SetCellValue("Sheet1", "A2", "软牡丹")
	_ = f.SetCellValue("Sheet1", "B2", "188")
	_ = f.SetCellValue("Sheet1", "A3", "硬中")
	_ = f.SetCellValue("Sheet1", "B3", "300")
	if err := f.SaveAs(pricePath); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s := &PriceService{}
	if _, err := s.LoadDefaultTemplate(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreviewPriceSheet(pricePath, "出"); err != nil {
		t.Fatal(err)
	}

	applied, err := s.ApplyPendingBatch()
	if err != nil {
		t.Fatal(err)
	}
	if applied.Updated != 2 {
		t.Fatalf("apply: %#v", applied)
	}

	var softIdx int
	for _, r := range applied.Rows {
		if r.Name == "软牡丹" && r.Status == "updated" {
			softIdx = r.Index
			if !r.CanUndo {
				t.Fatalf("soft should be undoable: %#v", r)
			}
		}
	}

	one, err := s.UndoBatchRow(softIdx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range one.Rows {
		if r.Name == "软牡丹" {
			found = true
			if r.Status != "undone" {
				t.Fatalf("expected undone: %#v", r)
			}
		}
	}
	if !found {
		t.Fatal("missing soft after undo")
	}

	all, err := s.UndoBatchAll()
	if err != nil {
		t.Fatal(err)
	}
	undone := 0
	for _, r := range all.Rows {
		if r.Status == "undone" {
			undone++
		}
		if r.Status == "updated" && r.CanUndo {
			t.Fatalf("still undoable after undo all: %#v", r)
		}
	}
	if undone < 2 {
		t.Fatalf("expected 2 undone, got %d %#v", undone, all)
	}
}
