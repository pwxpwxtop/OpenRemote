package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// 数据通道的二进制消息以一个字节的类型开头：
// 1 = 视频访问单元，2 = 文件分片（后面跟 uint32 LE 的传输 ID 和数据）。
const (
	msgTypeVideo byte = 1
	msgTypeFile  byte = 2

	fileChunkSize = 256 * 1024
)

// remoteFile 是远程目录里的一个文件，用于列表展示。
type remoteFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// fileBroker 处理控制端与被控端之间的文件收发。
// 被控端把收到的文件存进 ~/Downloads/OpenRemote，并按请求回传远端文件。
type fileBroker struct {
	conn *websocket.Conn

	mu      sync.Mutex
	uploads map[uint32]*uploadState
}

type uploadState struct {
	file *os.File
	name string
}

func newFileBroker(conn *websocket.Conn) *fileBroker {
	return &fileBroker{conn: conn, uploads: make(map[uint32]*uploadState)}
}

// handleEvent 处理控制端发来的文件控制事件。
func (b *fileBroker) handleEvent(ctx context.Context, ev inputEvent) {
	switch ev.Action {
	case "begin":
		b.beginUpload(ev)
	case "end":
		b.endUpload(ctx, ev)
	case "cancel":
		b.cancelUpload(ev)
	case "list":
		b.listFiles(ctx)
	case "get":
		go b.sendFile(ctx, ev)
	}
}

// handleChunk 把控制端上传的文件分片写入对应文件。
func (b *fileBroker) handleChunk(_ context.Context, data []byte) {
	if len(data) < 5 || data[0] != msgTypeFile {
		return
	}
	id := binary.LittleEndian.Uint32(data[1:5])
	payload := data[5:]

	b.mu.Lock()
	up := b.uploads[id]
	b.mu.Unlock()
	if up == nil {
		return
	}
	if _, err := up.file.Write(payload); err != nil {
		log.Printf("[file] write upload %d: %v", id, err)
		b.cancelUpload(inputEvent{ID: id})
	}
}

func (b *fileBroker) beginUpload(ev inputEvent) {
	dir, err := remoteFileDir()
	if err != nil {
		b.sendError(ev.ID, err.Error())
		return
	}
	name := uniqueFileName(dir, safeFileName(ev.Name))
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		b.sendError(ev.ID, fmt.Sprintf("创建文件失败: %v", err))
		return
	}

	b.mu.Lock()
	if old := b.uploads[ev.ID]; old != nil {
		_ = old.file.Close()
	}
	b.uploads[ev.ID] = &uploadState{file: file, name: name}
	b.mu.Unlock()
}

func (b *fileBroker) endUpload(ctx context.Context, ev inputEvent) {
	b.mu.Lock()
	up := b.uploads[ev.ID]
	delete(b.uploads, ev.ID)
	b.mu.Unlock()
	if up == nil {
		return
	}
	if err := up.file.Close(); err != nil {
		b.sendError(ev.ID, fmt.Sprintf("保存文件失败: %v", err))
		return
	}

	log.Printf("[file] received %s", up.name)
	_ = b.writeJSON(ctx, map[string]any{
		"event":  "file",
		"action": "done",
		"id":     ev.ID,
		"name":   up.name,
	})
}

func (b *fileBroker) cancelUpload(ev inputEvent) {
	b.mu.Lock()
	up := b.uploads[ev.ID]
	delete(b.uploads, ev.ID)
	b.mu.Unlock()
	if up != nil {
		_ = up.file.Close()
	}
}

func (b *fileBroker) listFiles(ctx context.Context) {
	dir, err := remoteFileDir()
	if err != nil {
		b.sendError(0, err.Error())
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		b.sendError(0, fmt.Sprintf("读取目录失败: %v", err))
		return
	}

	files := make([]remoteFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, remoteFile{Name: entry.Name(), Size: info.Size()})
	}

	_ = b.writeJSON(ctx, map[string]any{
		"event":  "file",
		"action": "list",
		"files":  files,
	})
}

// sendFile 把远端目录里的文件分片回传给控制端。
func (b *fileBroker) sendFile(ctx context.Context, ev inputEvent) {
	dir, err := remoteFileDir()
	if err != nil {
		b.sendError(ev.ID, err.Error())
		return
	}
	name := filepath.Base(ev.Name)
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		b.sendError(ev.ID, "文件不存在")
		return
	}

	file, err := os.Open(path)
	if err != nil {
		b.sendError(ev.ID, fmt.Sprintf("打开文件失败: %v", err))
		return
	}
	defer file.Close()

	if err := b.writeJSON(ctx, map[string]any{
		"event":  "file",
		"action": "begin",
		"id":     ev.ID,
		"name":   name,
		"size":   info.Size(),
	}); err != nil {
		return
	}

	buf := make([]byte, fileChunkSize)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if err := b.writeChunk(ctx, ev.ID, buf[:n]); err != nil {
				log.Printf("[file] send %s: %v", name, err)
				return
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			b.sendError(ev.ID, fmt.Sprintf("读取文件失败: %v", readErr))
			return
		}
	}

	_ = b.writeJSON(ctx, map[string]any{
		"event":  "file",
		"action": "end",
		"id":     ev.ID,
		"name":   name,
	})
}

func (b *fileBroker) writeJSON(ctx context.Context, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return b.conn.Write(writeCtx, websocket.MessageText, data)
}

func (b *fileBroker) writeChunk(ctx context.Context, id uint32, data []byte) error {
	buf := make([]byte, 5+len(data))
	buf[0] = msgTypeFile
	binary.LittleEndian.PutUint32(buf[1:5], id)
	copy(buf[5:], data)

	writeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.conn.Write(writeCtx, websocket.MessageBinary, buf)
}

func (b *fileBroker) sendError(id uint32, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = b.writeJSON(ctx, map[string]any{
		"event":   "file",
		"action":  "error",
		"id":      id,
		"message": message,
	})
}

// remoteFileDir 返回被控端的接收目录，不存在时自动创建。
func remoteFileDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法定位用户目录: %w", err)
	}
	dir := filepath.Join(home, "Downloads", "OpenRemote")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建接收目录失败: %w", err)
	}
	return dir, nil
}

// safeFileName 只保留文件名本身，并替换掉各平台不允许的字符。
func safeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	var builder strings.Builder
	for _, r := range name {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			builder.WriteRune('_')
			continue
		}
		builder.WriteRune(r)
	}
	out := strings.Trim(strings.TrimSpace(builder.String()), ".")
	if out == "" {
		return "file"
	}
	return out
}

// uniqueFileName 遇到同名文件时追加序号，避免覆盖已有文件。
func uniqueFileName(dir, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d%s", base, i, ext)
	}
}
