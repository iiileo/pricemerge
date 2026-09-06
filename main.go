package main

import (
	"embed"
	"log"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	priceService := NewPriceService()

	app := application.New(application.Options{
		Name:        "PriceMerge",
		Description: "出 / 接价格合并工具",
		Services: []application.Service{
			application.NewService(priceService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "PriceMerge",
		Width:          980,
		Height:         720,
		MinWidth:       880,
		MinHeight:      640,
		EnableFileDrop: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(232, 241, 244),
		URL:              "/",
	})

	win.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}
		xlsx := firstXLSX(files)
		if xlsx == "" {
			application.Get().Event.Emit("files-dropped", map[string]any{
				"error": "请拖入 .xlsx 文件",
			})
			return
		}
		role := ""
		if details := event.Context().DropTargetDetails(); details != nil {
			if details.Attributes != nil {
				role = details.Attributes["data-drop-role"]
			}
			if role == "" {
				role = details.ElementID
			}
		}
		application.Get().Event.Emit("files-dropped", map[string]any{
			"path": xlsx,
			"role": role,
			"name": filepath.Base(xlsx),
		})
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func firstXLSX(files []string) string {
	for _, f := range files {
		if strings.EqualFold(filepath.Ext(f), ".xlsx") {
			return f
		}
	}
	return ""
}
