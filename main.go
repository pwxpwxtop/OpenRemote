package main

import (
	"embed"
	_ "embed"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	setupLogging()
}

// setupLogging 把日志同时写到用户缓存目录下的文件，便于排查画面链路问题。
func setupLogging() {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "OpenRemote")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(dir, "openremote.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

func main() {
	hub := NewHubService()

	app := application.New(application.Options{
		Name:        "OpenRemote 1.0.0",
		Description: "Remote desktop control",
		Services: []application.Service{
			application.NewService(hub),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// HubService 需要 app 引用来向前端推送事件。
	hub.app = app

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "OpenRemote",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
			WebviewPreferences: application.MacWebviewPreferences{
				FullscreenEnabled: application.Enabled,
			},
		},
		BackgroundColour: application.NewRGB(15, 17, 21),
		URL:              "/",
		StartState:       application.WindowStateMaximised,
		MinWidth:         1024,
		MinHeight:        640,
		DisableResize:    false,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
