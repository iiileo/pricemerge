package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pricemerge-prefs-test-*")
	if err != nil {
		panic(err)
	}
	prefsFileOverride = filepath.Join(dir, "prefs.json")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestPersistAndRestoreBaseMapping(t *testing.T) {
	base := "3-16模版数据公式.xlsx"
	if _, err := os.Stat(base); err != nil {
		t.Skip("template missing")
	}
	mapPath := filepath.Join(t.TempDir(), "map.xlsx")
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "其他表名称")
	_ = f.SetCellValue("Sheet1", "B1", "基础表名称")
	_ = f.SetCellValue("Sheet1", "A2", "别名")
	_ = f.SetCellValue("Sheet1", "B2", "软牡丹")
	if err := f.SaveAs(mapPath); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s := &PriceService{}
	if _, err := s.LoadBase(base); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadMapping(mapPath); err != nil {
		t.Fatal(err)
	}
	p := loadPrefs()
	if p.BasePath == "" || p.MappingPath != mapPath {
		t.Fatalf("prefs not saved: %+v", p)
	}

	s2 := &PriceService{}
	st := s2.Status()
	if !st.Loaded || st.BlockCount == 0 {
		t.Fatalf("restore base failed: %+v", st)
	}
	if st.MappingCount != 1 {
		t.Fatalf("restore mapping count=%d prefs=%+v", st.MappingCount, loadPrefs())
	}

	s2.ClearMapping()
	p = loadPrefs()
	if p.MappingPath != "" {
		t.Fatalf("clear should drop mapping path: %+v", p)
	}
	if p.BasePath == "" {
		t.Fatal("clear mapping should keep base path")
	}
}
