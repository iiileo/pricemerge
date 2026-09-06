package main

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	kindOut = "出"
	kindIn  = "接"

	headerKindRow   = 4
	headerLabelRow  = 6
	dataStartRow    = 7
	porterHeader    = "搬运工"
	daqiHeader      = "大琪"
	diffHeader      = "差"
	defaultSheetIdx = 0
)

// priceBlock describes one 出/接 source block.
// Layout: 品名 | 搬运工 | 大琪(=搬运工-差) | 差
type priceBlock struct {
	Kind      string // 出 or 接
	Region    string
	NameCol   int // 1-based 品名
	PorterCol int // 1-based 搬运工（只读，不改）
	PriceCol  int // 1-based 大琪（目标展示价）
	DiffCol   int // 1-based 差
}

type nameHit struct {
	Kind       string
	Region     string
	Row        int
	NameCell   string
	PriceCell  string // 大琪
	PorterCell string
	DiffCell   string
	OldPrice   string
	Block      priceBlock
}

type workbookIndex struct {
	file   *excelize.File
	sheet  string
	blocks []priceBlock
	// nameIndex: kind -> name -> row hits within blocks
	nameIndex map[string]map[string][]nameLoc
}

type nameLoc struct {
	blockIdx int
	row      int
}

func openWorkbook(path string) (*workbookIndex, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开基础表失败: %w", err)
	}
	idx, err := indexWorkbook(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return idx, nil
}

func openWorkbookBytes(data []byte) (*workbookIndex, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("打开基础表失败: %w", err)
	}
	idx, err := indexWorkbook(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return idx, nil
}

func indexWorkbook(f *excelize.File) (*workbookIndex, error) {
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("工作簿没有工作表")
	}
	sheet := sheets[defaultSheetIdx]
	blocks, err := parsePriceBlocks(f, sheet)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("未解析到出/接价格区块（请确认第4行为出/接标记、第6行含「大琪」）")
	}
	idx := &workbookIndex{file: f, sheet: sheet, blocks: blocks}
	if err := idx.buildNameIndex(); err != nil {
		return nil, err
	}
	return idx, nil
}

func (w *workbookIndex) buildNameIndex() error {
	rows, err := w.file.GetRows(w.sheet)
	if err != nil {
		return err
	}
	w.nameIndex = make(map[string]map[string][]nameLoc, 2)
	for bi, b := range w.blocks {
		byName := w.nameIndex[b.Kind]
		if byName == nil {
			byName = make(map[string][]nameLoc)
			w.nameIndex[b.Kind] = byName
		}
		for r := dataStartRow; r <= len(rows); r++ {
			nameCell, _ := excelize.CoordinatesToCellName(b.NameCol, r)
			cellName, err := w.file.GetCellValue(w.sheet, nameCell)
			if err != nil {
				continue
			}
			cellName = strings.TrimSpace(cellName)
			if cellName == "" {
				continue
			}
			byName[cellName] = append(byName[cellName], nameLoc{blockIdx: bi, row: r})
		}
	}
	return nil
}

func parsePriceBlocks(f *excelize.File, sheet string) ([]priceBlock, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("读取工作表失败: %w", err)
	}
	if len(rows) < headerLabelRow {
		return nil, fmt.Errorf("工作表行数不足")
	}

	kindRow := rowCells(rows, headerKindRow)
	labelRow := rowCells(rows, headerLabelRow)

	type marker struct {
		col  int
		kind string
	}
	var markers []marker
	for col, val := range kindRow {
		v := strings.TrimSpace(val)
		if v == kindOut || v == kindIn {
			markers = append(markers, marker{col: col, kind: v})
		}
	}
	if len(markers) == 0 {
		return nil, fmt.Errorf("第%d行未找到「出」或「接」标记", headerKindRow)
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].col < markers[j].col })

	var porterCols []int
	maxCol := 0
	for col := range labelRow {
		if col > maxCol {
			maxCol = col
		}
	}
	for col := 1; col <= maxCol; col++ {
		if strings.TrimSpace(labelRow[col]) == porterHeader {
			porterCols = append(porterCols, col)
		}
	}

	var blocks []priceBlock
	for i, m := range markers {
		next := maxCol + 2
		if i+1 < len(markers) {
			next = markers[i+1].col
		}
		var inRange []int
		for _, pc := range porterCols {
			if pc > m.col && pc < next {
				inRange = append(inRange, pc)
			}
		}
		if len(inRange) == 0 {
			continue
		}

		add := func(kind string, porterCol int) {
			nameCol := porterCol - 1
			daqiCol := porterCol + 1
			if strings.TrimSpace(labelRow[daqiCol]) != daqiHeader {
				found := 0
				for c := porterCol + 1; c <= porterCol+3 && c <= maxCol; c++ {
					if strings.TrimSpace(labelRow[c]) == daqiHeader {
						found = c
						break
					}
				}
				if found == 0 {
					return
				}
				daqiCol = found
			}
			diffCol := daqiCol + 1
			if strings.TrimSpace(labelRow[diffCol]) == daqiHeader ||
				strings.TrimSpace(labelRow[diffCol]) == porterHeader ||
				strings.TrimSpace(labelRow[diffCol]) == "公司价" {
				// Unusual layout; try scan.
				found := 0
				for c := daqiCol + 1; c <= daqiCol+3 && c <= maxCol; c++ {
					if strings.TrimSpace(labelRow[c]) == diffHeader {
						found = c
						break
					}
				}
				if found != 0 {
					diffCol = found
				}
			}
			// 接区块的「差」表头经常为空，但仍占大琪右侧一列（公式 =搬运工-差）。
			region := strings.TrimSpace(labelRow[nameCol])
			blocks = append(blocks, priceBlock{
				Kind:      kind,
				Region:    region,
				NameCol:   nameCol,
				PorterCol: porterCol,
				PriceCol:  daqiCol,
				DiffCol:   diffCol,
			})
		}

		switch m.kind {
		case kindOut:
			add(kindOut, inRange[0])
			// Fujian-style: 出 marker covers both 出 and 接 when next marker is also 出.
			if len(inRange) >= 2 && (i+1 >= len(markers) || markers[i+1].kind == kindOut) {
				add(kindIn, inRange[1])
			}
		case kindIn:
			add(kindIn, inRange[0])
		}
	}
	return blocks, nil
}

func rowCells(rows [][]string, row1Based int) map[int]string {
	out := map[int]string{}
	if row1Based < 1 || row1Based > len(rows) {
		return out
	}
	row := rows[row1Based-1]
	for i, v := range row {
		out[i+1] = v
	}
	return out
}

func (w *workbookIndex) close() error {
	if w == nil || w.file == nil {
		return nil
	}
	return w.file.Close()
}

func (w *workbookIndex) findByName(kind, name string) ([]nameHit, error) {
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(kind)
	if name == "" {
		return nil, fmt.Errorf("名称为空")
	}
	if kind != kindOut && kind != kindIn {
		return nil, fmt.Errorf("类型必须是「出」或「接」")
	}

	locs := w.nameIndex[kind][name]
	if len(locs) == 0 {
		return nil, nil
	}

	hits := make([]nameHit, 0, len(locs))
	for _, loc := range locs {
		b := w.blocks[loc.blockIdx]
		r := loc.row
		nameCell, _ := excelize.CoordinatesToCellName(b.NameCol, r)
		priceCell, _ := excelize.CoordinatesToCellName(b.PriceCol, r)
		porterCell, _ := excelize.CoordinatesToCellName(b.PorterCol, r)
		diffCell := ""
		if b.DiffCol > 0 {
			diffCell, _ = excelize.CoordinatesToCellName(b.DiffCol, r)
		}
		old := w.readDaqiPrice(priceCell, porterCell, diffCell)
		hits = append(hits, nameHit{
			Kind:       b.Kind,
			Region:     b.Region,
			Row:        r,
			NameCell:   nameCell,
			PriceCell:  priceCell,
			PorterCell: porterCell,
			DiffCell:   diffCell,
			OldPrice:   old,
			Block:      b,
		})
	}
	return hits, nil
}

// readDaqiPrice returns the effective 大琪 price.
// Formula cells often have no/stale cached value after UpdateLinkedValue on export,
// so when 大琪 is 搬运工-差 we compute it directly.
func (w *workbookIndex) readDaqiPrice(priceCell, porterCell, diffCell string) string {
	formula, _ := w.file.GetCellFormula(w.sheet, priceCell)
	formula = strings.TrimSpace(formula)
	if formula != "" && diffCell != "" {
		porterVal, _ := w.file.GetCellValue(w.sheet, porterCell)
		diffVal, _ := w.file.GetCellValue(w.sheet, diffCell)
		porter, ok := parseFloat(porterVal)
		if !ok {
			porter = 0
		}
		diff, ok := parseFloat(diffVal)
		if !ok {
			diff = 0
		}
		// Skip pure-empty rows (section headers) that happen to sit in a price column.
		if strings.TrimSpace(porterVal) == "" && strings.TrimSpace(diffVal) == "" {
			if cached, _ := w.file.GetCellValue(w.sheet, priceCell); strings.TrimSpace(cached) != "" {
				return strings.TrimSpace(cached)
			}
			return ""
		}
		return strconv.FormatFloat(porter-diff, 'f', -1, 64)
	}

	val, _ := w.file.GetCellValue(w.sheet, priceCell)
	val = strings.TrimSpace(val)
	if val != "" {
		return val
	}
	if formula != "" {
		if calc, err := w.file.CalcCellValue(w.sheet, priceCell); err == nil {
			if calc = strings.TrimSpace(calc); calc != "" {
				return calc
			}
		}
	}
	return ""
}

// applyDaqiPrice sets the effective 大琪 price without touching 搬运工.
// If 大琪 has formula (=搬运工-差), adjust 差 so the formula still yields price.
// If 大琪 is a plain value, write price into 大琪 directly.
func (w *workbookIndex) applyDaqiPrice(h nameHit, price float64) (string, error) {
	formula, err := w.file.GetCellFormula(w.sheet, h.PriceCell)
	if err != nil {
		return "", err
	}
	formula = strings.TrimSpace(formula)

	if formula != "" {
		if h.DiffCell == "" {
			return "", fmt.Errorf("%s 有公式但未找到「差」列", h.PriceCell)
		}
		porterVal, err := w.file.GetCellValue(w.sheet, h.PorterCell)
		if err != nil {
			return "", err
		}
		porter, ok := parseFloat(porterVal)
		if !ok {
			// 搬运工为空/非数字时按 0 处理，只改「差」，保留大琪公式
			porter = 0
		}
		diff := porter - price
		if err := w.file.SetCellValue(w.sheet, h.DiffCell, diff); err != nil {
			return "", err
		}
		if err := w.file.SetCellFormula(w.sheet, h.PriceCell, formula); err != nil {
			return "", err
		}
		detail := fmt.Sprintf("%s(大琪公式保留) %s差=%v", h.PriceCell, h.DiffCell, diff)
		if !ok {
			detail += fmt.Sprintf(" %s搬运工空按0", h.PorterCell)
		}
		return detail, nil
	}

	if err := w.file.SetCellValue(w.sheet, h.PriceCell, price); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s=%v", h.PriceCell, price), nil
}

func parseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func (w *workbookIndex) saveAs(path string) error {
	// Drop stale formula caches so left-side refs (=大琪) recalculate on open.
	if err := w.file.UpdateLinkedValue(); err != nil {
		return err
	}
	return w.file.SaveAs(path)
}
