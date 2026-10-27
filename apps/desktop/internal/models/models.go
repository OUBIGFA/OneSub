// Package models consumes OneAsr's exact model IDs, file sizes and hashes.
package models

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed catalog.json
var catalogJSON []byte

type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Files []File `json:"files"`
}

func Catalog() []Model {
	var c struct {
		Models []Model `json:"models"`
	}
	if err := json.Unmarshal(catalogJSON, &c); err != nil {
		panic(err)
	}
	return c.Models
}
func ByID(id string) (Model, error) {
	for _, m := range Catalog() {
		if m.ID == id {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("未知模型：%s", id)
}
func (m Model) Size() int64 {
	var size int64
	for _, f := range m.Files {
		size += f.Size
	}
	return size
}
func Verify(ctx context.Context, m Model, dir string) error {
	for _, f := range m.Files {
		if err := verifyFile(ctx, filepath.Join(dir, f.Name), f); err != nil {
			return err
		}
	}
	return nil
}
func verifyFile(ctx context.Context, path string, expected File) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("模型文件未就绪：%s", expected.Name)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != expected.Size {
		return fmt.Errorf("模型文件大小不符：%s", expected.Name)
	}
	hash := sha256.New()
	buf := make([]byte, 1024*1024)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := file.Read(buf)
		if n > 0 {
			hash.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("模型校验失败：%s", expected.Name)
	}
	return nil
}

type Progress struct {
	Done, Total int64
	Stage       string
}

// Download resumes only the requested model's partials. A file becomes visible
// under its final name only after exact length and SHA-256 validation.
func Download(ctx context.Context, client *http.Client, m Model, dir string, progress func(Progress)) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	var completed int64
	for _, spec := range m.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		final := filepath.Join(dir, spec.Name)
		if err := verifyFile(ctx, final, spec); err == nil {
			completed += spec.Size
			progress(Progress{completed, m.Size(), "已校验"})
			continue
		}
		partial := final + ".partial"
		var offset int64
		if info, err := os.Stat(partial); err == nil {
			offset = info.Size()
		}
		if offset > spec.Size {
			return fmt.Errorf("下载片段大小异常：%s；请重新选择空目录", spec.Name)
		}
		if offset < spec.Size {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
			if err != nil {
				return err
			}
			if offset > 0 {
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
			}
			response, err := client.Do(req)
			if err != nil {
				return err
			}
			if response.StatusCode != 200 && response.StatusCode != 206 {
				response.Body.Close()
				return fmt.Errorf("模型下载失败 HTTP %d", response.StatusCode)
			}
			if response.StatusCode == 200 {
				offset = 0
			} else if !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(offset, 10)+"-") {
				response.Body.Close()
				return errors.New("续传响应范围不符，未追加数据")
			}
			flag := os.O_CREATE | os.O_WRONLY
			if offset == 0 {
				flag |= os.O_TRUNC
			} else {
				flag |= os.O_APPEND
			}
			file, err := os.OpenFile(partial, flag, 0600)
			if err != nil {
				response.Body.Close()
				return err
			}
			buf := make([]byte, 256*1024)
			last := time.Time{}
			for {
				n, readErr := response.Body.Read(buf)
				if offset+int64(n) > spec.Size {
					err = errors.New("模型响应超过登记大小")
					break
				}
				if n > 0 {
					var written int
					written, err = file.Write(buf[:n])
					offset += int64(written)
					if err != nil {
						break
					}
				}
				if time.Since(last) > 200*time.Millisecond {
					progress(Progress{completed + offset, m.Size(), "下载中"})
					last = time.Now()
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					err = readErr
					break
				}
			}
			if syncErr := file.Sync(); err == nil {
				err = syncErr
			}
			closeErr := file.Close()
			response.Body.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
		}
		progress(Progress{completed + offset, m.Size(), "校验中"})
		if err := verifyFile(ctx, partial, spec); err != nil {
			return err
		}
		if err := os.Rename(partial, final); err != nil {
			return err
		}
		completed += spec.Size
	}
	progress(Progress{m.Size(), m.Size(), "已校验"})
	return nil
}
