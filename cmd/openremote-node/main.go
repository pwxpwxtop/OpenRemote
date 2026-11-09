package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"changeme/internal/node"
)

const version = "1.5.0"

func main() {
	addr := flag.String("addr", ":9000", "监听地址，例如 :9000")
	publicURL := flag.String("public-url", "", "对外可访问的基础地址（走反向代理时使用，例如 https://remote.example.com）")
	showVersion := flag.Bool("version", false, "打印版本号后退出")
	flag.Parse()

	node.Version = version

	if *showVersion {
		fmt.Println("openremote-node", version)
		return
	}

	hub := node.NewHub(*publicURL)

	server := &http.Server{
		Addr:              *addr,
		Handler:           hub.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[node] openremote-node %s listening on %s", version, *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[node] serve error: %v", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Println("[node] stopped")
}
