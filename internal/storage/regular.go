package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrFileTooLarge 表示常规文件体积超过调用方允许的最大字节上限。
var ErrFileTooLarge = errors.New("file exceeds size limit")

// RegularFile 抽象已打开的常规文件句柄，便于上层在单元测试中验证句柄生命周期。
type RegularFile interface {
	io.Reader
	io.Closer
	Stat() (os.FileInfo, error)
}

// OpenRegularNoFollow 以不跟随符号链接与重解析点的方式原子只读打开常规文件，
// 拒绝目录、管道、符号链接与重解析点（无 Lstat/open 竞态窗口）。
func OpenRegularNoFollow(path string) (*os.File, error) {
	return openNoFollow(path)
}

// ReadOpenRegularFileBounded 校验已打开句柄指向常规文件且体积不超过 maxBytes，
// 读取完整内容并返回文件元信息。
func ReadOpenRegularFileBounded(f RegularFile, maxBytes int64) ([]byte, os.FileInfo, error) {
	if maxBytes < 0 {
		return nil, nil, os.ErrInvalid
	}
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, os.ErrInvalid
	}
	if info.Size() > maxBytes {
		return nil, nil, fmt.Errorf("%w: %w", os.ErrInvalid, ErrFileTooLarge)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, nil, fmt.Errorf("%w: %w", os.ErrInvalid, ErrFileTooLarge)
	}
	return data, info, nil
}

// ReadRegularFileBounded 拒绝符号链接、重解析点与非常规文件，有界读取内容并在返回前关闭句柄，
// 避免 Windows 在后续原子重命名替换文件时因读句柄占用返回访问拒绝。
func ReadRegularFileBounded(path string, maxBytes int64) ([]byte, os.FileInfo, error) {
	if maxBytes < 0 {
		return nil, nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	f, err := OpenRegularNoFollow(path)
	if err != nil {
		return nil, nil, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	data, finfo, err := ReadOpenRegularFileBounded(f, maxBytes)
	if err != nil {
		return nil, nil, &os.PathError{Op: "read", Path: path, Err: err}
	}
	closed = true
	if err := f.Close(); err != nil {
		return nil, nil, err
	}
	return data, finfo, nil
}
