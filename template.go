package main

import (
	_ "embed"
	"fmt"
)

//go:embed 3-16模版数据公式.xlsx
var embeddedTemplate []byte

func defaultTemplateBytes() ([]byte, error) {
	if len(embeddedTemplate) == 0 {
		return nil, fmt.Errorf("内置模版为空")
	}
	return embeddedTemplate, nil
}
